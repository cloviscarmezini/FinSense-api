package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"finsense-api/internal/database"
	"finsense-api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type syncHeadCacheEntry struct {
	latestRevision int64
	lastClientID   string
	expiresAt      time.Time
}

type SyncService struct {
	db        *database.DB
	headMu    sync.RWMutex
	headCache map[uuid.UUID]syncHeadCacheEntry
}

func NewSyncService(db *database.DB) *SyncService {
	return &SyncService{
		db:        db,
		headCache: make(map[uuid.UUID]syncHeadCacheEntry),
	}
}

func (s *SyncService) invalidateHeadCache(userID uuid.UUID) {
	s.headMu.Lock()
	delete(s.headCache, userID)
	s.headMu.Unlock()
}

func (s *SyncService) getHeadCache(userID uuid.UUID) (syncHeadCacheEntry, bool) {
	s.headMu.RLock()
	entry, ok := s.headCache[userID]
	s.headMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return syncHeadCacheEntry{}, false
	}
	return entry, true
}

func (s *SyncService) setHeadCache(userID uuid.UUID, latestRevision int64, lastClientID string) {
	s.headMu.Lock()
	s.headCache[userID] = syncHeadCacheEntry{
		latestRevision: latestRevision,
		lastClientID:   lastClientID,
		expiresAt:      time.Now().Add(5 * time.Second),
	}
	s.headMu.Unlock()
}

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	return strings.ToLower(matchAllCap.ReplaceAllString(snake, "${1}_${2}"))
}

func isDateField(k string) bool {
	return strings.HasSuffix(k, "_date") || k == "date" || strings.HasSuffix(k, "_at")
}

func normalizeEntityData(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	res := make(map[string]interface{}, len(data))
	for k, v := range data {
		snakeKey := toSnakeCase(k)
		if isDateField(snakeKey) {
			if num, ok := v.(float64); ok {
				var t time.Time
				if num > 1e11 {
					t = time.UnixMilli(int64(num)).UTC()
				} else if num > 1e9 {
					t = time.Unix(int64(num), 0).UTC()
				} else {
					// Apple Cocoa epoch (seconds since Jan 1 2001)
					t = time.Unix(int64(num)+978307200, 0).UTC()
				}
				v = t.Format(time.RFC3339)
			}
		}
		res[snakeKey] = v
	}
	return res
}

func entityPriority(entityName string, op models.OperationType) int {
	if op == models.OpDelete {
		switch entityName {
		case "financial_transactions", "amortization_plans":
			return 1
		case "credit_card_invoices", "category_quota_rules", "category_provisions", "credit_contracts", "investment_items", "debt_items":
			return 2
		case "credit_cards", "categories":
			return 3
		case "bank_accounts":
			return 4
		default:
			return 5
		}
	} else {
		switch entityName {
		case "bank_accounts":
			return 1
		case "credit_cards", "categories":
			return 2
		case "credit_card_invoices", "credit_contracts", "investment_items", "debt_items":
			return 3
		case "category_provisions", "category_quota_rules", "amortization_plans", "sinking_funds", "what_if_scenarios", "financial_insights", "monthly_reviews", "ml_behavioral_profiles":
			return 4
		case "financial_transactions":
			return 5
		default:
			return 6
		}
	}
}

func sortMutationsByDependency(muts []models.Mutation) {
	sort.SliceStable(muts, func(i, j int) bool {
		pi := entityPriority(muts[i].EntityName, muts[i].Operation)
		pj := entityPriority(muts[j].EntityName, muts[j].Operation)
		return pi < pj
	})
}

// PushPull handles processing client mutations and returning server deltas
func (s *SyncService) PushPull(ctx context.Context, userID uuid.UUID, req models.PushPullRequest) (*models.PushPullResponse, error) {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	sinceRev := req.SinceRevision
	if sinceRev == 0 && req.ClientLastSyncRevision > 0 {
		sinceRev = req.ClientLastSyncRevision
	}

	// Sort mutations to enforce dependency order (e.g. Accounts and Categories before Transactions)
	sortMutationsByDependency(req.Mutations)

	appliedIDs := make([]uuid.UUID, 0, len(req.Mutations))
	ackIDs := make([]uuid.UUID, 0, len(req.Mutations))
	rejectedMutations := make([]models.RejectedMutation, 0)
	skippedMutations := make([]models.Mutation, 0)

	for _, m := range req.Mutations {
		// Normalization of fields
		if m.Data == nil && m.Payload != nil {
			m.Data = m.Payload
		}
		m.Data = normalizeEntityData(m.Data)

		if m.Timestamp.IsZero() && m.UpdatedAt != nil {
			m.Timestamp = *m.UpdatedAt
		}
		opStr := string(m.Operation)
		if opStr == "UPSERT" {
			opStr = "INSERT"
			m.Operation = models.OpInsert
		}

		// Check idempotency
		var exists bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sync_changelog WHERE mutation_id = $1)", m.MutationID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("failed to check mutation idempotency: %w", err)
		}

		if exists {
			ackIDs = append(ackIDs, m.MutationID)
			appliedIDs = append(appliedIDs, m.MutationID)
			continue
		}

		// Savepoint per mutation to isolate individual failures
		_, err = tx.Exec(ctx, "SAVEPOINT mut_sp")
		if err != nil {
			return nil, fmt.Errorf("failed to create savepoint: %w", err)
		}

		// Apply mutation based on entity type and operation with Last-Write-Wins (LWW)
		applied, err := s.applyMutation(ctx, tx, userID, m)
		if err != nil {
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT mut_sp")
			rejectedMutations = append(rejectedMutations, models.RejectedMutation{
				MutationID: m.MutationID,
				Reason:     err.Error(),
			})
			continue
		}

		if applied {
			// Record in changelog with valid constraint check (INSERT, UPDATE, DELETE)
			_, err = tx.Exec(ctx, `
				INSERT INTO sync_changelog (user_id, client_id, entity_name, entity_id, operation, mutation_id, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, userID, req.ClientID, m.EntityName, m.EntityID, opStr, m.MutationID, time.Now().UTC())
			if err != nil {
				_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT mut_sp")
				rejectedMutations = append(rejectedMutations, models.RejectedMutation{
					MutationID: m.MutationID,
					Reason:     err.Error(),
				})
				continue
			}
		} else {
			// Mutation was superseded by a newer operation (LWW conflict resolution).
			// We track it to immediately return the winning current state to the client.
			skippedMutations = append(skippedMutations, m)
		}

		_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT mut_sp")
		appliedIDs = append(appliedIDs, m.MutationID)
		ackIDs = append(ackIDs, m.MutationID)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	if len(appliedIDs) > 0 {
		s.invalidateHeadCache(userID)
	}

	// Fetch server changes since since_revision (excluding the mutations just applied in this request)
	serverChanges, latestRev, deltas, tombstones, err := s.GetDeltaWithContainers(ctx, userID, appliedIDs, sinceRev)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server changes: %w", err)
	}

	// For any skipped mutations (stale operations that lost LWW), ensure the client receives the winning entity state
	for _, m := range skippedMutations {
		var isDeleted bool
		var updatedAt time.Time
		query := fmt.Sprintf("SELECT deleted_at IS NOT NULL, updated_at FROM %s WHERE id = $1 AND user_id = $2", m.EntityName)
		rowErr := s.db.Pool.QueryRow(ctx, query, m.EntityID, userID).Scan(&isDeleted, &updatedAt)
		if rowErr != nil {
			continue
		}
		if isDeleted {
			tombstones = append(tombstones, map[string]interface{}{
				"entity_name": m.EntityName,
				"entity_id":   m.EntityID,
				"deleted_at":  updatedAt.Format(time.RFC3339),
			})
			serverChanges = append(serverChanges, models.Mutation{
				MutationID: uuid.New(),
				EntityName: m.EntityName,
				EntityID:   m.EntityID,
				Operation:  models.OpDelete,
				Timestamp:  updatedAt,
				UpdatedAt:  &updatedAt,
			})
		} else {
			entityData, _ := s.fetchEntityData(ctx, userID, m.EntityName, m.EntityID)
			if entityData != nil {
				appendToDeltas(deltas, m.EntityName, entityData)
				serverChanges = append(serverChanges, models.Mutation{
					MutationID: uuid.New(),
					EntityName: m.EntityName,
					EntityID:   m.EntityID,
					Operation:  models.OpInsert,
					Data:       entityData,
					Timestamp:  updatedAt,
					UpdatedAt:  &updatedAt,
				})
			}
		}
	}

	go s.touchDeviceSession(context.Background(), userID, req.ClientID)

	return &models.PushPullResponse{
		LatestRevision:          latestRev,
		ServerCurrentRevision:   latestRev,
		AppliedMutationIDs:      appliedIDs,
		AcknowledgedMutationIDs: ackIDs,
		RejectedMutations:       rejectedMutations,
		ServerChanges:           serverChanges,
		Deltas:                  deltas,
		Tombstones:              tombstones,
	}, nil
}

func (s *SyncService) touchDeviceSession(ctx context.Context, userID uuid.UUID, clientID string) {
	if clientID == "" || clientID == "api" {
		return
	}
	platform := "Unknown"
	lower := strings.ToLower(clientID)
	if strings.Contains(lower, "mac") {
		platform = "macOS"
	} else if strings.Contains(lower, "ios") || strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") {
		platform = "iOS"
	}
	_, _ = s.db.Pool.Exec(ctx, `
		INSERT INTO user_device_sessions (user_id, device_id, platform, app_version, last_access_at, updated_at)
		VALUES ($1, $2, $3, '1.0.0', NOW(), NOW())
		ON CONFLICT (user_id, device_id) DO UPDATE SET
			last_access_at = NOW(),
			updated_at = NOW()
	`, userID, clientID, platform)
}

func initEmptyDeltas() map[string]interface{} {
	return map[string]interface{}{
		"bank_accounts":          []interface{}{},
		"bankAccounts":           []interface{}{},
		"credit_cards":           []interface{}{},
		"creditCards":            []interface{}{},
		"credit_card_invoices":   []interface{}{},
		"creditCardInvoices":     []interface{}{},
		"categories":             []interface{}{},
		"financial_transactions": []interface{}{},
		"financialTransactions":  []interface{}{},
		"investment_items":       []interface{}{},
		"investmentItems":        []interface{}{},
		"debt_items":             []interface{}{},
		"debtItems":              []interface{}{},
		"credit_contracts":       []interface{}{},
		"creditContracts":        []interface{}{},
		"amortization_plans":     []interface{}{},
		"amortizationPlans":      []interface{}{},
		"category_provisions":    []interface{}{},
		"categoryProvisions":     []interface{}{},
		"category_quota_rules":   []interface{}{},
		"categoryQuotaRules":     []interface{}{},
		"sinking_funds":          []interface{}{},
		"sinkingFunds":           []interface{}{},
		"what_if_scenarios":      []interface{}{},
		"whatIfScenarios":        []interface{}{},
		"financial_insights":     []interface{}{},
		"financialInsights":      []interface{}{},
		"monthly_reviews":        []interface{}{},
		"monthlyReviews":         []interface{}{},
		"ml_behavioral_profiles": []interface{}{},
		"mlBehavioralProfiles":   []interface{}{},
	}
}

func appendToDeltas(deltas map[string]interface{}, entityName string, data map[string]interface{}) {
	switch entityName {
	case "bank_accounts":
		deltas["bank_accounts"] = append(deltas["bank_accounts"].([]interface{}), data)
		deltas["bankAccounts"] = append(deltas["bankAccounts"].([]interface{}), data)
	case "credit_cards":
		deltas["credit_cards"] = append(deltas["credit_cards"].([]interface{}), data)
		deltas["creditCards"] = append(deltas["creditCards"].([]interface{}), data)
	case "credit_card_invoices":
		deltas["credit_card_invoices"] = append(deltas["credit_card_invoices"].([]interface{}), data)
		deltas["creditCardInvoices"] = append(deltas["creditCardInvoices"].([]interface{}), data)
	case "categories":
		deltas["categories"] = append(deltas["categories"].([]interface{}), data)
	case "financial_transactions":
		deltas["financial_transactions"] = append(deltas["financial_transactions"].([]interface{}), data)
		deltas["financialTransactions"] = append(deltas["financialTransactions"].([]interface{}), data)
	case "investment_items":
		deltas["investment_items"] = append(deltas["investment_items"].([]interface{}), data)
		deltas["investmentItems"] = append(deltas["investmentItems"].([]interface{}), data)
	case "debt_items":
		deltas["debt_items"] = append(deltas["debt_items"].([]interface{}), data)
		deltas["debtItems"] = append(deltas["debtItems"].([]interface{}), data)
	case "credit_contracts":
		deltas["credit_contracts"] = append(deltas["credit_contracts"].([]interface{}), data)
		deltas["creditContracts"] = append(deltas["creditContracts"].([]interface{}), data)
	case "amortization_plans":
		deltas["amortization_plans"] = append(deltas["amortization_plans"].([]interface{}), data)
		deltas["amortizationPlans"] = append(deltas["amortizationPlans"].([]interface{}), data)
	case "category_provisions":
		deltas["category_provisions"] = append(deltas["category_provisions"].([]interface{}), data)
		deltas["categoryProvisions"] = append(deltas["categoryProvisions"].([]interface{}), data)
	case "category_quota_rules":
		deltas["category_quota_rules"] = append(deltas["category_quota_rules"].([]interface{}), data)
		deltas["categoryQuotaRules"] = append(deltas["categoryQuotaRules"].([]interface{}), data)
	case "sinking_funds":
		deltas["sinking_funds"] = append(deltas["sinking_funds"].([]interface{}), data)
		deltas["sinkingFunds"] = append(deltas["sinkingFunds"].([]interface{}), data)
	case "what_if_scenarios":
		deltas["what_if_scenarios"] = append(deltas["what_if_scenarios"].([]interface{}), data)
		deltas["whatIfScenarios"] = append(deltas["whatIfScenarios"].([]interface{}), data)
	case "financial_insights":
		deltas["financial_insights"] = append(deltas["financial_insights"].([]interface{}), data)
		deltas["financialInsights"] = append(deltas["financialInsights"].([]interface{}), data)
	case "monthly_reviews":
		deltas["monthly_reviews"] = append(deltas["monthly_reviews"].([]interface{}), data)
		deltas["monthlyReviews"] = append(deltas["monthlyReviews"].([]interface{}), data)
	case "ml_behavioral_profiles":
		deltas["ml_behavioral_profiles"] = append(deltas["ml_behavioral_profiles"].([]interface{}), data)
		deltas["mlBehavioralProfiles"] = append(deltas["mlBehavioralProfiles"].([]interface{}), data)
	}
}

// GetAllActiveEntities returns full active snapshot when since_revision == 0 (fresh start / restore)
func (s *SyncService) GetAllActiveEntities(ctx context.Context, userID uuid.UUID) ([]models.Mutation, int64, map[string]interface{}, []map[string]interface{}, error) {
	var currentLatest int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COALESCE(MAX(revision), 0) FROM sync_changelog WHERE user_id = $1", userID).Scan(&currentLatest)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	tables := []string{
		"bank_accounts",
		"credit_cards",
		"credit_card_invoices",
		"categories",
		"financial_transactions",
		"investment_items",
		"debt_items",
		"credit_contracts",
		"amortization_plans",
		"category_provisions",
		"category_quota_rules",
		"sinking_funds",
		"what_if_scenarios",
		"financial_insights",
		"monthly_reviews",
		"ml_behavioral_profiles",
	}

	var changes []models.Mutation
	deltas := initEmptyDeltas()
	tombstones := make([]map[string]interface{}, 0)

	for _, table := range tables {
		query := fmt.Sprintf("SELECT id, updated_at, row_to_json(t) FROM (SELECT * FROM %s WHERE user_id = $1 AND deleted_at IS NULL) t", table)
		rows, err := s.db.Pool.Query(ctx, query, userID)
		if err != nil {
			return nil, 0, nil, nil, fmt.Errorf("failed to query active entities from %s: %w", table, err)
		}
		for rows.Next() {
			var id uuid.UUID
			var updatedAt time.Time
			var jsonStr string
			if err := rows.Scan(&id, &updatedAt, &jsonStr); err != nil {
				rows.Close()
				return nil, 0, nil, nil, err
			}
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
				continue
			}
			data = normalizeEntityData(data)
			changes = append(changes, models.Mutation{
				MutationID: uuid.New(),
				EntityName: table,
				EntityID:   id,
				Operation:  models.OpInsert,
				Data:       data,
				Timestamp:  updatedAt,
				UpdatedAt:  &updatedAt,
			})
			appendToDeltas(deltas, table, data)
		}
		rows.Close()
	}

	return changes, currentLatest, deltas, tombstones, nil
}

// GetDeltaWithContainers returns changelog records since since_revision without filtering by client_id
func (s *SyncService) GetDeltaWithContainers(ctx context.Context, userID uuid.UUID, excludeMutationIDs []uuid.UUID, sinceRev int64) ([]models.Mutation, int64, map[string]interface{}, []map[string]interface{}, error) {
	if sinceRev == 0 {
		return s.GetAllActiveEntities(ctx, userID)
	}

	var currentLatest int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COALESCE(MAX(revision), 0) FROM sync_changelog WHERE user_id = $1", userID).Scan(&currentLatest)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	query := `
		SELECT revision, entity_name, entity_id, operation, mutation_id, created_at
		FROM sync_changelog
		WHERE user_id = $1 AND revision > $2
		ORDER BY revision ASC
		LIMIT 1000
	`

	rows, err := s.db.Pool.Query(ctx, query, userID, sinceRev)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	defer rows.Close()

	excludeMap := make(map[uuid.UUID]bool, len(excludeMutationIDs))
	for _, id := range excludeMutationIDs {
		excludeMap[id] = true
	}

	var changes []models.Mutation
	deltas := initEmptyDeltas()
	tombstones := make([]map[string]interface{}, 0)

	for rows.Next() {
		var rev int64
		var entityName string
		var entityID uuid.UUID
		var opStr string
		var mutID uuid.UUID
		var createdAt time.Time

		if err := rows.Scan(&rev, &entityName, &entityID, &opStr, &mutID, &createdAt); err != nil {
			return nil, 0, nil, nil, err
		}

		if excludeMap[mutID] {
			continue
		}

		var entityData map[string]interface{}
		if opStr == "DELETE" {
			tombstones = append(tombstones, map[string]interface{}{
				"entity_name": entityName,
				"entity_id":   entityID,
				"deleted_at":  createdAt.Format(time.RFC3339),
			})
		} else {
			entityData, _ = s.fetchEntityData(ctx, userID, entityName, entityID)
			if entityData == nil {
				opStr = "DELETE"
				tombstones = append(tombstones, map[string]interface{}{
					"entity_name": entityName,
					"entity_id":   entityID,
					"deleted_at":  createdAt.Format(time.RFC3339),
				})
			} else {
				appendToDeltas(deltas, entityName, entityData)
			}
		}

		changes = append(changes, models.Mutation{
			MutationID: mutID,
			EntityName: entityName,
			EntityID:   entityID,
			Operation:  models.OperationType(opStr),
			Data:       entityData,
			Timestamp:  createdAt,
			UpdatedAt:  &createdAt,
		})
	}

	return changes, currentLatest, deltas, tombstones, nil
}

// GetDelta returns changelog records since since_revision (retro-compatible)
func (s *SyncService) GetDelta(ctx context.Context, userID uuid.UUID, clientID string, sinceRev int64) ([]models.Mutation, int64, error) {
	changes, currentLatest, _, _, err := s.GetDeltaWithContainers(ctx, userID, nil, sinceRev)
	return changes, currentLatest, err
}

// CheckSyncStatus returns an O(1) summary indicating whether the cloud has revisions newer than sinceRev for other clients.
func (s *SyncService) CheckSyncStatus(ctx context.Context, userID uuid.UUID, clientID string, sinceRev int64) (*models.SyncStatusResponse, error) {
	if cached, ok := s.getHeadCache(userID); ok {
		serverReset := sinceRev > 0 && cached.latestRevision < sinceRev
		if serverReset {
			return &models.SyncStatusResponse{
				LatestRevision: cached.latestRevision,
				HasChanges:     true,
				ServerReset:    true,
			}, nil
		}
		if cached.latestRevision <= sinceRev {
			return &models.SyncStatusResponse{
				LatestRevision: cached.latestRevision,
				HasChanges:     false,
				ServerReset:    false,
			}, nil
		}
		if clientID == "" || cached.lastClientID != clientID {
			return &models.SyncStatusResponse{
				LatestRevision: cached.latestRevision,
				HasChanges:     true,
				ServerReset:    false,
			}, nil
		}
	}

	var latestRev int64
	var lastClientID string
	err := s.db.Pool.QueryRow(ctx, `
		SELECT revision, client_id
		FROM sync_changelog
		WHERE user_id = $1
		ORDER BY revision DESC
		LIMIT 1
	`, userID).Scan(&latestRev, &lastClientID)
	if err != nil {
		if err == pgx.ErrNoRows {
			latestRev = 0
			lastClientID = ""
		} else {
			return nil, fmt.Errorf("failed to query sync head: %w", err)
		}
	}

	s.setHeadCache(userID, latestRev, lastClientID)

	serverReset := sinceRev > 0 && latestRev < sinceRev
	hasChanges := false
	if serverReset {
		hasChanges = true
	} else if latestRev > sinceRev {
		if clientID == "" || lastClientID != clientID {
			hasChanges = true
		} else {
			// Latest revision came from this same client; verify in O(1) whether another device wrote between sinceRev and latestRev
			var remoteExists bool
			existsErr := s.db.Pool.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1
					FROM sync_changelog
					WHERE user_id = $1 AND revision > $2 AND client_id <> $3
				)
			`, userID, sinceRev, clientID).Scan(&remoteExists)
			if existsErr != nil {
				return nil, fmt.Errorf("failed to check remote changes: %w", existsErr)
			}
			hasChanges = remoteExists
		}
	}

	return &models.SyncStatusResponse{
		LatestRevision: latestRev,
		HasChanges:     hasChanges,
		ServerReset:    serverReset,
	}, nil
}

// getOrFallbackAccount returns accountID if valid, or falls back to user's first account, or creates a default one
func (s *SyncService) getOrFallbackAccount(ctx context.Context, tx pgx.Tx, userID uuid.UUID, accountID uuid.UUID) (uuid.UUID, error) {
	if accountID != uuid.Nil {
		var exists bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM bank_accounts WHERE id = $1 AND user_id = $2)", accountID, userID).Scan(&exists)
		if err == nil && exists {
			return accountID, nil
		}
	}
	var fallbackID uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM bank_accounts WHERE user_id = $1 AND deleted_at IS NULL ORDER BY display_order ASC, created_at ASC LIMIT 1", userID).Scan(&fallbackID)
	if err == nil {
		return fallbackID, nil
	}

	fallbackID = uuid.New()
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO bank_accounts (id, user_id, name, bank_preset_id, icon_symbol, color_hex, initial_balance, agency, account_number, display_order, created_at, updated_at)
		VALUES ($1, $2, 'Conta Principal', 'itau', 'building.columns.fill', '#EC7000', 0, '', '', 0, $3, $3)
	`, fallbackID, userID, now)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create fallback account: %w", err)
	}
	return fallbackID, nil
}

// getOrFallbackCategory returns categoryID if valid, or falls back to user's category matching type, or creates a default one
func (s *SyncService) getOrFallbackCategory(ctx context.Context, tx pgx.Tx, userID uuid.UUID, categoryID uuid.UUID, catType string) (uuid.UUID, error) {
	if categoryID != uuid.Nil {
		var exists bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)", categoryID, userID).Scan(&exists)
		if err == nil && exists {
			return categoryID, nil
		}
	}
	if catType == "" {
		catType = "expense"
	}
	var fallbackID uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM categories WHERE user_id = $1 AND type = $2 AND deleted_at IS NULL ORDER BY display_order ASC, created_at ASC LIMIT 1", userID, catType).Scan(&fallbackID)
	if err == nil {
		return fallbackID, nil
	}

	err = tx.QueryRow(ctx, "SELECT id FROM categories WHERE user_id = $1 AND deleted_at IS NULL ORDER BY display_order ASC, created_at ASC LIMIT 1", userID).Scan(&fallbackID)
	if err == nil {
		return fallbackID, nil
	}

	fallbackID = uuid.New()
	now := time.Now().UTC()
	catName := "Geral"
	if catType == "income" {
		catName = "Outras Receitas"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO categories (id, user_id, name, type, icon_symbol, color_hex, is_system_default, display_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'tag.fill', '#6366F1', true, 0, $5, $5)
	`, fallbackID, userID, catName, catType, now)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create fallback category: %w", err)
	}
	return fallbackID, nil
}

// applyMutation executes insert/update/delete according to entity name with Last-Write-Wins (LWW)
func (s *SyncService) applyMutation(ctx context.Context, tx pgx.Tx, userID uuid.UUID, m models.Mutation) (bool, error) {
	effectiveTime := m.Timestamp.UTC()
	if effectiveTime.IsZero() {
		if m.UpdatedAt != nil && !m.UpdatedAt.IsZero() {
			effectiveTime = m.UpdatedAt.UTC()
		} else {
			effectiveTime = time.Now().UTC()
		}
	}

	validTables := map[string]bool{
		"bank_accounts":          true,
		"credit_cards":           true,
		"credit_card_invoices":   true,
		"categories":             true,
		"financial_transactions": true,
		"debt_items":             true,
		"investment_items":       true,
		"credit_contracts":       true,
		"amortization_plans":     true,
		"category_provisions":    true,
		"category_quota_rules":   true,
		"sinking_funds":          true,
		"what_if_scenarios":      true,
		"financial_insights":     true,
		"monthly_reviews":        true,
		"ml_behavioral_profiles": true,
	}
	if !validTables[m.EntityName] {
		return false, nil
	}

	if m.Operation == models.OpDelete {
		tag, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE %s 
			SET deleted_at = $1, updated_at = $1 
			WHERE id = $2 AND user_id = $3 AND (updated_at <= $1 OR updated_at IS NULL)
		`, m.EntityName), effectiveTime, m.EntityID, userID)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	}

	jsonBytes, _ := json.Marshal(m.Data)

	switch m.EntityName {
	case "bank_accounts":
		var item models.BankAccount
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}

		query := `
			INSERT INTO bank_accounts (id, user_id, name, bank_preset_id, custom_image_path, icon_symbol, color_hex, initial_balance, agency, account_number, display_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				bank_preset_id = EXCLUDED.bank_preset_id,
				custom_image_path = EXCLUDED.custom_image_path,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				initial_balance = EXCLUDED.initial_balance,
				agency = EXCLUDED.agency,
				account_number = EXCLUDED.account_number,
				display_order = EXCLUDED.display_order,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE bank_accounts.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.UserID, item.Name, item.BankPresetID, item.CustomImagePath, item.IconSymbol, item.ColorHex, item.InitialBalance, item.Agency, item.AccountNumber, item.DisplayOrder, createdAt, effectiveTime)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "credit_cards":
		var item models.CreditCard
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		if item.ClosingDay < 1 || item.ClosingDay > 31 {
			item.ClosingDay = 25
		}
		if item.BestPurchaseDay < 1 || item.BestPurchaseDay > 31 {
			if item.ClosingDay >= 31 {
				item.BestPurchaseDay = 1
			} else {
				item.BestPurchaseDay = item.ClosingDay + 1
			}
		}
		if item.DueDay < 1 || item.DueDay > 31 {
			item.DueDay = 5
		}
		if item.Brand == "" {
			item.Brand = "Mastercard"
		}
		if item.IconSymbol == "" {
			item.IconSymbol = "creditcard.fill"
		}
		if item.ColorHex == "" {
			item.ColorHex = "#820AD1"
		}
		if item.DefaultPaymentAccountID != nil && *item.DefaultPaymentAccountID != uuid.Nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM bank_accounts WHERE id = $1 AND user_id = $2)", *item.DefaultPaymentAccountID, userID).Scan(&exists)
			if !exists {
				item.DefaultPaymentAccountID = nil
			}
		} else {
			item.DefaultPaymentAccountID = nil
		}

		query := `
			INSERT INTO credit_cards (
				id, user_id, name, total_limit, closing_day, best_purchase_day, due_day,
				default_payment_account_id, brand, bank_preset_id, custom_image_path,
				icon_symbol, color_hex, last_four_digits, display_order, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				total_limit = EXCLUDED.total_limit,
				closing_day = EXCLUDED.closing_day,
				best_purchase_day = EXCLUDED.best_purchase_day,
				due_day = EXCLUDED.due_day,
				default_payment_account_id = EXCLUDED.default_payment_account_id,
				brand = EXCLUDED.brand,
				bank_preset_id = EXCLUDED.bank_preset_id,
				custom_image_path = EXCLUDED.custom_image_path,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				last_four_digits = EXCLUDED.last_four_digits,
				display_order = EXCLUDED.display_order,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE credit_cards.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.Name, item.TotalLimit, item.ClosingDay, item.BestPurchaseDay, item.DueDay,
			item.DefaultPaymentAccountID, item.Brand, item.BankPresetID, item.CustomImagePath,
			item.IconSymbol, item.ColorHex, item.LastFourDigits, item.DisplayOrder, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "credit_card_invoices":
		var item models.CreditCardInvoice
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		var cardExists bool
		_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM credit_cards WHERE id = $1 AND user_id = $2)", item.CreditCardID, userID).Scan(&cardExists)
		if !cardExists {
			return false, fmt.Errorf("credit_card_id %s not found for invoice", item.CreditCardID)
		}
		if item.PaidFromAccountID != nil && *item.PaidFromAccountID != uuid.Nil {
			var accExists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM bank_accounts WHERE id = $1 AND user_id = $2)", *item.PaidFromAccountID, userID).Scan(&accExists)
			if !accExists {
				item.PaidFromAccountID = nil
			}
		} else {
			item.PaidFromAccountID = nil
		}

		query := `
			INSERT INTO credit_card_invoices (
				id, user_id, credit_card_id, year, month, closing_date, due_date,
				is_paid, paid_amount, paid_date, paid_from_account_id, payment_transaction_id,
				created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (id) DO UPDATE SET
				credit_card_id = EXCLUDED.credit_card_id,
				year = EXCLUDED.year,
				month = EXCLUDED.month,
				closing_date = EXCLUDED.closing_date,
				due_date = EXCLUDED.due_date,
				is_paid = EXCLUDED.is_paid,
				paid_amount = EXCLUDED.paid_amount,
				paid_date = EXCLUDED.paid_date,
				paid_from_account_id = EXCLUDED.paid_from_account_id,
				payment_transaction_id = EXCLUDED.payment_transaction_id,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE credit_card_invoices.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.CreditCardID, item.Year, item.Month, item.ClosingDate, item.DueDate,
			item.IsPaid, item.PaidAmount, item.PaidDate, item.PaidFromAccountID, item.PaymentTransactionID,
			createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "categories":
		var item models.Category
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}

		query := `
			INSERT INTO categories (id, user_id, name, type, icon_symbol, color_hex, keywords, is_system_default, display_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				type = EXCLUDED.type,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				keywords = EXCLUDED.keywords,
				is_system_default = EXCLUDED.is_system_default,
				display_order = EXCLUDED.display_order,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE categories.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.UserID, item.Name, item.Type, item.IconSymbol, item.ColorHex, item.Keywords, item.IsSystemDefault, item.DisplayOrder, createdAt, effectiveTime)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "financial_transactions":
		var item models.FinancialTransaction
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		if item.RecurrenceMode == "" {
			item.RecurrenceMode = "single"
		}
		if item.StatementOccurrenceIndex == 0 {
			item.StatementOccurrenceIndex = 1
		}

		isCardPurchase := item.CreditCardID != nil && *item.CreditCardID != uuid.Nil && (item.TransactionOrigin == nil || *item.TransactionOrigin != "creditCardInvoicePayment")
		if isCardPurchase {
			item.AccountID = *item.CreditCardID
		} else {
			accID, err := s.getOrFallbackAccount(ctx, tx, userID, item.AccountID)
			if err != nil {
				return false, fmt.Errorf("account validation failed: %w", err)
			}
			item.AccountID = accID
		}

		catID, err := s.getOrFallbackCategory(ctx, tx, userID, item.CategoryID, item.Type)
		if err != nil {
			return false, fmt.Errorf("category validation failed: %w", err)
		}
		item.CategoryID = catID

		if item.LinkedContractID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM credit_contracts WHERE id = $1 AND user_id = $2)", *item.LinkedContractID, userID).Scan(&exists)
			if !exists {
				item.LinkedContractID = nil
			}
		}
		if item.LinkedInvestmentID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM investment_items WHERE id = $1 AND user_id = $2)", *item.LinkedInvestmentID, userID).Scan(&exists)
			if !exists {
				item.LinkedInvestmentID = nil
			}
		}
		if item.LinkedDebtID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM debt_items WHERE id = $1 AND user_id = $2)", *item.LinkedDebtID, userID).Scan(&exists)
			if !exists {
				item.LinkedDebtID = nil
			}
		}
		if item.LinkedQuotaID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM category_quota_rules WHERE (id = $1 OR series_id = $1) AND user_id = $2)", *item.LinkedQuotaID, userID).Scan(&exists)
			if !exists {
				item.LinkedQuotaID = nil
			}
		}

		query := `
			INSERT INTO financial_transactions (
				id, user_id, account_id, category_id, name, details, amount, date, type, is_paid,
				attachment_path, attachment_name, ignore_in_consolidated, recurrence_mode, recurrence_frequency,
				is_canceled_recurring_series, series_group_id, installment_index, installment_total,
				raw_statement_line, statement_occurrence_index, linked_contract_id, linked_investment_id,
				linked_debt_id, linked_quota_id, linked_quota_units, linked_quota_year_month_key,
				transaction_origin, linked_entity_id,
				base_installment_registered_amount, projected_installment_correction_amount, confirmed_real_interest_correction_amount,
				credit_card_id, credit_card_invoice_year_month_key, credit_card_invoice_id,
				created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37)
			ON CONFLICT (id) DO UPDATE SET
				account_id = EXCLUDED.account_id,
				category_id = EXCLUDED.category_id,
				name = EXCLUDED.name,
				details = EXCLUDED.details,
				amount = EXCLUDED.amount,
				date = EXCLUDED.date,
				type = EXCLUDED.type,
				is_paid = EXCLUDED.is_paid,
				attachment_path = EXCLUDED.attachment_path,
				attachment_name = EXCLUDED.attachment_name,
				ignore_in_consolidated = EXCLUDED.ignore_in_consolidated,
				recurrence_mode = EXCLUDED.recurrence_mode,
				recurrence_frequency = EXCLUDED.recurrence_frequency,
				is_canceled_recurring_series = EXCLUDED.is_canceled_recurring_series,
				series_group_id = EXCLUDED.series_group_id,
				installment_index = EXCLUDED.installment_index,
				installment_total = EXCLUDED.installment_total,
				raw_statement_line = EXCLUDED.raw_statement_line,
				statement_occurrence_index = EXCLUDED.statement_occurrence_index,
				linked_contract_id = EXCLUDED.linked_contract_id,
				linked_investment_id = EXCLUDED.linked_investment_id,
				linked_debt_id = EXCLUDED.linked_debt_id,
				linked_quota_id = EXCLUDED.linked_quota_id,
				linked_quota_units = EXCLUDED.linked_quota_units,
				linked_quota_year_month_key = EXCLUDED.linked_quota_year_month_key,
				transaction_origin = EXCLUDED.transaction_origin,
				linked_entity_id = EXCLUDED.linked_entity_id,
				base_installment_registered_amount = EXCLUDED.base_installment_registered_amount,
				projected_installment_correction_amount = EXCLUDED.projected_installment_correction_amount,
				confirmed_real_interest_correction_amount = EXCLUDED.confirmed_real_interest_correction_amount,
				credit_card_id = EXCLUDED.credit_card_id,
				credit_card_invoice_year_month_key = EXCLUDED.credit_card_invoice_year_month_key,
				credit_card_invoice_id = EXCLUDED.credit_card_invoice_id,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE financial_transactions.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.AccountID, item.CategoryID, item.Name, item.Details, item.Amount, item.Date, item.Type, item.IsPaid,
			item.AttachmentPath, item.AttachmentName, item.IgnoreInConsolidated, item.RecurrenceMode, item.RecurrenceFrequency,
			item.IsCanceledRecurringSeries, item.SeriesGroupID, item.InstallmentIndex, item.InstallmentTotal,
			item.RawStatementLine, item.StatementOccurrenceIndex, item.LinkedContractID, item.LinkedInvestmentID,
			item.LinkedDebtID, item.LinkedQuotaID, item.LinkedQuotaUnits, item.LinkedQuotaYearMonthKey,
			item.TransactionOrigin, item.LinkedEntityID,
			item.BaseInstallmentRegisteredAmount, item.ProjectedInstallmentCorrectionAmount, item.ConfirmedRealInterestCorrectionAmount,
			item.CreditCardID, item.CreditCardInvoiceYearMonthKey, item.CreditCardInvoiceID,
			createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "debt_items":
		var item models.DebtItem
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		if item.PaidFromAccountID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM bank_accounts WHERE id = $1 AND user_id = $2)", *item.PaidFromAccountID, userID).Scan(&exists)
			if !exists {
				item.PaidFromAccountID = nil
			}
		}
		if item.PaymentTransactionID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM financial_transactions WHERE id = $1 AND user_id = $2)", *item.PaymentTransactionID, userID).Scan(&exists)
			if !exists {
				item.PaymentTransactionID = nil
			}
		}
		query := `
			INSERT INTO debt_items (id, user_id, name, custom_image_path, bank_preset_id, icon_symbol, color_hex, original_amount, contracted_date, notes, is_paid, negotiated_amount, paid_date, paid_from_account_id, payment_transaction_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				custom_image_path = EXCLUDED.custom_image_path,
				bank_preset_id = EXCLUDED.bank_preset_id,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				original_amount = EXCLUDED.original_amount,
				contracted_date = EXCLUDED.contracted_date,
				notes = EXCLUDED.notes,
				is_paid = EXCLUDED.is_paid,
				negotiated_amount = EXCLUDED.negotiated_amount,
				paid_date = EXCLUDED.paid_date,
				paid_from_account_id = EXCLUDED.paid_from_account_id,
				payment_transaction_id = EXCLUDED.payment_transaction_id,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE debt_items.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.UserID, item.Name, item.CustomImagePath, item.BankPresetID, item.IconSymbol, item.ColorHex, item.OriginalAmount, item.ContractedDate, item.Notes, item.IsPaid, item.NegotiatedAmount, item.PaidDate, item.PaidFromAccountID, item.PaymentTransactionID, createdAt, effectiveTime)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "investment_items":
		var item models.InvestmentItem
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		accID, err := s.getOrFallbackAccount(ctx, tx, userID, item.InvestmentAccountID)
		if err != nil {
			return false, fmt.Errorf("investment account validation failed: %w", err)
		}
		item.InvestmentAccountID = accID

		var movementsJSON []byte
		if item.Movements != nil {
			movementsJSON, _ = json.Marshal(item.Movements)
		}

		query := `
			INSERT INTO investment_items (id, user_id, investment_account_id, name, group_name, balance, show_in_consolidated, icon_symbol, color_hex, notes, movements, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			ON CONFLICT (id) DO UPDATE SET
				investment_account_id = EXCLUDED.investment_account_id,
				name = EXCLUDED.name,
				group_name = EXCLUDED.group_name,
				balance = EXCLUDED.balance,
				show_in_consolidated = EXCLUDED.show_in_consolidated,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				notes = EXCLUDED.notes,
				movements = EXCLUDED.movements,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE investment_items.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.UserID, item.InvestmentAccountID, item.Name, item.GroupName, item.Balance, item.ShowInConsolidated, item.IconSymbol, item.ColorHex, item.Notes, movementsJSON, createdAt, effectiveTime)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "credit_contracts":
		var item models.CreditContract
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		accID, err := s.getOrFallbackAccount(ctx, tx, userID, item.AccountID)
		if err != nil {
			return false, fmt.Errorf("credit contract account validation failed: %w", err)
		}
		item.AccountID = accID

		catID, err := s.getOrFallbackCategory(ctx, tx, userID, item.CategoryID, "expense")
		if err != nil {
			return false, fmt.Errorf("credit contract category validation failed: %w", err)
		}
		item.CategoryID = catID

		if item.SettledAccountID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM bank_accounts WHERE id = $1 AND user_id = $2)", *item.SettledAccountID, userID).Scan(&exists)
			if !exists {
				item.SettledAccountID = nil
			}
		}

		var financingDetailsJSON, loanDetailsJSON, amortizationHistoryJSON, scheduledInstallmentsJSON []byte
		if item.FinancingDetails != nil {
			financingDetailsJSON, _ = json.Marshal(item.FinancingDetails)
		}
		if item.LoanDetails != nil {
			loanDetailsJSON, _ = json.Marshal(item.LoanDetails)
		}
		if item.AmortizationHistory != nil {
			amortizationHistoryJSON, _ = json.Marshal(item.AmortizationHistory)
		}
		if item.ScheduledInstallments != nil {
			scheduledInstallmentsJSON, _ = json.Marshal(item.ScheduledInstallments)
		}

		query := `
			INSERT INTO credit_contracts (
				id, user_id, account_id, category_id, kind, name, contract_number, loan_subtype, financing_subtype,
				amortization_system, total_contracted_amount, outstanding_balance, installment_amount, billing_day,
				total_installments, original_total_installments, monthly_interest_rate, annual_nominal_interest_rate,
				indexation_rate, admin_fee, insurance_fee, base_installment_amount, financing_details, loan_details,
				amortization_history, start_date, past_installments_elapsed_at_creation, extra_amortized_total,
				is_settled, settled_amount, settled_date, settled_account_id,
				bank_due_day, reference_date, maturity_date, estimated_monthly_tr, last_fgts_usage_date,
				scheduled_installments, average_monthly_installment_growth,
				notes, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41, $42)
			ON CONFLICT (id) DO UPDATE SET
				account_id = EXCLUDED.account_id,
				category_id = EXCLUDED.category_id,
				kind = EXCLUDED.kind,
				name = EXCLUDED.name,
				contract_number = EXCLUDED.contract_number,
				loan_subtype = EXCLUDED.loan_subtype,
				financing_subtype = EXCLUDED.financing_subtype,
				amortization_system = EXCLUDED.amortization_system,
				total_contracted_amount = EXCLUDED.total_contracted_amount,
				outstanding_balance = EXCLUDED.outstanding_balance,
				installment_amount = EXCLUDED.installment_amount,
				billing_day = EXCLUDED.billing_day,
				total_installments = EXCLUDED.total_installments,
				original_total_installments = EXCLUDED.original_total_installments,
				monthly_interest_rate = EXCLUDED.monthly_interest_rate,
				annual_nominal_interest_rate = EXCLUDED.annual_nominal_interest_rate,
				indexation_rate = EXCLUDED.indexation_rate,
				admin_fee = EXCLUDED.admin_fee,
				insurance_fee = EXCLUDED.insurance_fee,
				base_installment_amount = EXCLUDED.base_installment_amount,
				financing_details = EXCLUDED.financing_details,
				loan_details = EXCLUDED.loan_details,
				amortization_history = EXCLUDED.amortization_history,
				start_date = EXCLUDED.start_date,
				past_installments_elapsed_at_creation = EXCLUDED.past_installments_elapsed_at_creation,
				extra_amortized_total = EXCLUDED.extra_amortized_total,
				is_settled = EXCLUDED.is_settled,
				settled_amount = EXCLUDED.settled_amount,
				settled_date = EXCLUDED.settled_date,
				settled_account_id = EXCLUDED.settled_account_id,
				bank_due_day = EXCLUDED.bank_due_day,
				reference_date = EXCLUDED.reference_date,
				maturity_date = EXCLUDED.maturity_date,
				estimated_monthly_tr = EXCLUDED.estimated_monthly_tr,
				last_fgts_usage_date = EXCLUDED.last_fgts_usage_date,
				scheduled_installments = EXCLUDED.scheduled_installments,
				average_monthly_installment_growth = EXCLUDED.average_monthly_installment_growth,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE credit_contracts.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.AccountID, item.CategoryID, item.Kind, item.Name, item.ContractNumber, item.LoanSubtype, item.FinancingSubtype,
			item.AmortizationSystem, item.TotalContractedAmount, item.OutstandingBalance, item.InstallmentAmount, item.BillingDay,
			item.TotalInstallments, item.OriginalTotalInstallments, item.MonthlyInterestRate, item.AnnualNominalInterestRate,
			item.IndexationRate, item.AdminFee, item.InsuranceFee, item.BaseInstallmentAmount, financingDetailsJSON, loanDetailsJSON,
			amortizationHistoryJSON, item.StartDate, item.PastInstallmentsElapsedAtCreation, item.ExtraAmortizedTotal,
			item.IsSettled, item.SettledAmount, item.SettledDate, item.SettledAccountID,
			item.BankDueDay, item.ReferenceDate, item.MaturityDate, item.EstimatedMonthlyTR, item.LastFGTSUsageDate,
			scheduledInstallmentsJSON, item.AverageMonthlyInstallmentGrowth,
			item.Notes, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "amortization_plans":
		var item models.AmortizationPlan
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		if item.GenerationMode == "" {
			item.GenerationMode = "Manual"
		}
		if item.BillingDay < 1 || item.BillingDay > 31 {
			item.BillingDay = 26
		}
		if item.IncomeCommitmentPercentage <= 0 {
			item.IncomeCommitmentPercentage = 30.0
		}
		if item.DefaultRecalculationMode == "" {
			item.DefaultRecalculationMode = "Reduzir Prazo (Quantidade de Parcelas)"
		}

		// Verify contract exists for user
		var contractExists bool
		_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM credit_contracts WHERE id = $1 AND user_id = $2)", item.ContractID, userID).Scan(&contractExists)
		if !contractExists {
			return false, nil
		}

		var itemsJSON []byte
		if item.Items != nil {
			itemsJSON, _ = json.Marshal(item.Items)
		}

		query := `
			INSERT INTO amortization_plans (
				id, user_id, contract_id, generation_mode, billing_day,
				income_commitment_percentage, default_recalculation_mode, estimated_monthly_tr,
				items, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO UPDATE SET
				contract_id = EXCLUDED.contract_id,
				generation_mode = EXCLUDED.generation_mode,
				billing_day = EXCLUDED.billing_day,
				income_commitment_percentage = EXCLUDED.income_commitment_percentage,
				default_recalculation_mode = EXCLUDED.default_recalculation_mode,
				estimated_monthly_tr = EXCLUDED.estimated_monthly_tr,
				items = EXCLUDED.items,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE amortization_plans.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.ContractID, item.GenerationMode, item.BillingDay,
			item.IncomeCommitmentPercentage, item.DefaultRecalculationMode, item.EstimatedMonthlyTR,
			itemsJSON, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "amortization_records":
		var item models.AmortizationRecord
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		accID, err := s.getOrFallbackAccount(ctx, tx, userID, item.AccountID)
		if err != nil {
			return false, fmt.Errorf("amortization record account validation failed: %w", err)
		}
		item.AccountID = accID
		if item.TransactionID != nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM financial_transactions WHERE id = $1 AND user_id = $2)", *item.TransactionID, userID).Scan(&exists)
			if !exists {
				item.TransactionID = nil
			}
		}

		query := `
			INSERT INTO amortization_records (
				id, user_id, contract_id, account_id, transaction_id, date, amount_paid, mode,
				previous_installment_amount, new_installment_amount, previous_remaining_installments,
				new_remaining_installments, effective_amortization, previous_balance, notes, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			ON CONFLICT (id) DO UPDATE SET
				contract_id = EXCLUDED.contract_id,
				account_id = EXCLUDED.account_id,
				transaction_id = EXCLUDED.transaction_id,
				date = EXCLUDED.date,
				amount_paid = EXCLUDED.amount_paid,
				mode = EXCLUDED.mode,
				previous_installment_amount = EXCLUDED.previous_installment_amount,
				new_installment_amount = EXCLUDED.new_installment_amount,
				previous_remaining_installments = EXCLUDED.previous_remaining_installments,
				new_remaining_installments = EXCLUDED.new_remaining_installments,
				effective_amortization = EXCLUDED.effective_amortization,
				previous_balance = EXCLUDED.previous_balance,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE amortization_records.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.ContractID, item.AccountID, item.TransactionID, item.Date, item.AmountPaid, item.Mode,
			item.PreviousInstallmentAmount, item.NewInstallmentAmount, item.PreviousRemainingInstallments,
			item.NewRemainingInstallments, item.EffectiveAmortization, item.PreviousBalance, item.Notes, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "category_provisions":
		var item models.CategoryProvision
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		catID, err := s.getOrFallbackCategory(ctx, tx, userID, item.CategoryID, "expense")
		if err != nil {
			return false, fmt.Errorf("provision category validation failed: %w", err)
		}
		item.CategoryID = catID

		query := `
			INSERT INTO category_provisions (id, user_id, category_id, year, month, estimated_amount, repeat_for_future_months, monthly_overrides, excluded_year_month_keys, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO UPDATE SET
				category_id = EXCLUDED.category_id,
				year = EXCLUDED.year,
				month = EXCLUDED.month,
				estimated_amount = EXCLUDED.estimated_amount,
				repeat_for_future_months = EXCLUDED.repeat_for_future_months,
				monthly_overrides = EXCLUDED.monthly_overrides,
				excluded_year_month_keys = EXCLUDED.excluded_year_month_keys,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE category_provisions.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.UserID, item.CategoryID, item.Year, item.Month, item.EstimatedAmount, item.RepeatForFutureMonths, item.MonthlyOverrides, item.ExcludedYearMonthKeys, createdAt, effectiveTime)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "category_quota_rules":
		var item models.CategoryQuotaRule
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}

		quotaType := item.QuotaType
		if quotaType == "" {
			quotaType = "expense"
		}
		item.QuotaType = quotaType

		var categoryID *uuid.UUID
		if item.CategoryID != nil && *item.CategoryID != uuid.Nil {
			catID, err := s.getOrFallbackCategory(ctx, tx, userID, *item.CategoryID, "expense")
			if err == nil {
				categoryID = &catID
			}
		} else if quotaType == "expense" {
			catID, err := s.getOrFallbackCategory(ctx, tx, userID, uuid.Nil, "expense")
			if err != nil {
				return false, fmt.Errorf("quota category validation failed: %w", err)
			}
			categoryID = &catID
		}
		item.CategoryID = categoryID

		if item.LinkedInvestmentID != nil && *item.LinkedInvestmentID != uuid.Nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM investment_items WHERE id = $1 AND user_id = $2)", *item.LinkedInvestmentID, userID).Scan(&exists)
			if !exists {
				item.LinkedInvestmentID = nil
			}
		} else {
			item.LinkedInvestmentID = nil
		}

		query := `
			INSERT INTO category_quota_rules (
				id, series_id, user_id, category_id, name, icon_symbol, year, month,
				planned_quantity, estimated_unit_price, repeat_for_future_months,
				monthly_overrides, excluded_year_month_keys, quota_type, linked_investment_id,
				created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
			ON CONFLICT (id) DO UPDATE SET
				series_id = EXCLUDED.series_id,
				category_id = EXCLUDED.category_id,
				name = EXCLUDED.name,
				icon_symbol = EXCLUDED.icon_symbol,
				year = EXCLUDED.year,
				month = EXCLUDED.month,
				planned_quantity = EXCLUDED.planned_quantity,
				estimated_unit_price = EXCLUDED.estimated_unit_price,
				repeat_for_future_months = EXCLUDED.repeat_for_future_months,
				monthly_overrides = EXCLUDED.monthly_overrides,
				excluded_year_month_keys = EXCLUDED.excluded_year_month_keys,
				quota_type = EXCLUDED.quota_type,
				linked_investment_id = EXCLUDED.linked_investment_id,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE category_quota_rules.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.SeriesID, item.UserID, item.CategoryID, item.Name, item.IconSymbol,
			item.Year, item.Month, item.PlannedQuantity, item.EstimatedUnitPrice,
			item.RepeatForFutureMonths, item.MonthlyOverrides, item.ExcludedYearMonthKeys,
			item.QuotaType, item.LinkedInvestmentID, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "sinking_funds":
		var item models.SinkingFund
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		if item.RecurrenceMonths < 1 {
			item.RecurrenceMonths = 12
		}
		if item.DueMonth < 1 || item.DueMonth > 12 {
			item.DueMonth = 1
		}
		if item.DueYear < 2020 {
			item.DueYear = 2027
		}
		if item.CategoryID != nil && *item.CategoryID != uuid.Nil {
			catID, err := s.getOrFallbackCategory(ctx, tx, userID, *item.CategoryID, "expense")
			if err == nil {
				item.CategoryID = &catID
			} else {
				item.CategoryID = nil
			}
		} else {
			item.CategoryID = nil
		}
		query := `
			INSERT INTO sinking_funds (
				id, user_id, name, category_id, icon_symbol, color_hex,
				target_amount, recurrence_months, due_month, due_year, accumulated_reserve,
				is_enabled, notes, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				category_id = EXCLUDED.category_id,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				target_amount = EXCLUDED.target_amount,
				recurrence_months = EXCLUDED.recurrence_months,
				due_month = EXCLUDED.due_month,
				due_year = EXCLUDED.due_year,
				accumulated_reserve = EXCLUDED.accumulated_reserve,
				is_enabled = EXCLUDED.is_enabled,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE sinking_funds.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.Name, item.CategoryID, item.IconSymbol, item.ColorHex,
			item.TargetAmount, item.RecurrenceMonths, item.DueMonth, item.DueYear, item.AccumulatedReserve,
			item.IsEnabled, item.Notes, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "what_if_scenarios":
		var item models.WhatIfScenario
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		query := `
			INSERT INTO what_if_scenarios (
				id, user_id, name, variable_income_reduction_percent, unexpected_expense_amount,
				unexpected_expense_month_offset, benchmark_annual_rate_percent, net_yield_tax_rate_percent,
				notes, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				variable_income_reduction_percent = EXCLUDED.variable_income_reduction_percent,
				unexpected_expense_amount = EXCLUDED.unexpected_expense_amount,
				unexpected_expense_month_offset = EXCLUDED.unexpected_expense_month_offset,
				benchmark_annual_rate_percent = EXCLUDED.benchmark_annual_rate_percent,
				net_yield_tax_rate_percent = EXCLUDED.net_yield_tax_rate_percent,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE what_if_scenarios.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.Name, item.VariableIncomeReductionPercent, item.UnexpectedExpenseAmount,
			item.UnexpectedExpenseMonthOffset, item.BenchmarkAnnualRatePercent, item.NetYieldTaxRatePercent,
			item.Notes, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "financial_insights":
		var item models.FinancialInsight
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		if item.RelatedCategoryID != nil && *item.RelatedCategoryID != uuid.Nil {
			var exists bool
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)", *item.RelatedCategoryID, userID).Scan(&exists)
			if !exists {
				item.RelatedCategoryID = nil
			}
		} else {
			item.RelatedCategoryID = nil
		}
		query := `
			INSERT INTO financial_insights (
				id, user_id, fingerprint, kind, severity, title, message, recommendation,
				impact_amount, confidence_score, year_month_key, related_category_id, related_transaction_id,
				feedback, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (id) DO UPDATE SET
				fingerprint = EXCLUDED.fingerprint,
				kind = EXCLUDED.kind,
				severity = EXCLUDED.severity,
				title = EXCLUDED.title,
				message = EXCLUDED.message,
				recommendation = EXCLUDED.recommendation,
				impact_amount = EXCLUDED.impact_amount,
				confidence_score = EXCLUDED.confidence_score,
				year_month_key = EXCLUDED.year_month_key,
				related_category_id = EXCLUDED.related_category_id,
				related_transaction_id = EXCLUDED.related_transaction_id,
				feedback = EXCLUDED.feedback,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE financial_insights.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.Fingerprint, item.Kind, item.Severity, item.Title, item.Message, item.Recommendation,
			item.ImpactAmount, item.ConfidenceScore, item.YearMonthKey, item.RelatedCategoryID, item.RelatedTransactionID,
			item.Feedback, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "monthly_reviews":
		var item models.MonthlyReview
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		query := `
			INSERT INTO monthly_reviews (
				id, user_id, year, month, planned_income, realized_income, planned_expense, realized_expense,
				planned_end_balance, realized_end_balance, previous_month_end_balance,
				pending_amortizations_count, pending_amortizations_amount, deviation_highlights,
				is_completed, completed_at, notes, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
			ON CONFLICT (id) DO UPDATE SET
				year = EXCLUDED.year,
				month = EXCLUDED.month,
				planned_income = EXCLUDED.planned_income,
				realized_income = EXCLUDED.realized_income,
				planned_expense = EXCLUDED.planned_expense,
				realized_expense = EXCLUDED.realized_expense,
				planned_end_balance = EXCLUDED.planned_end_balance,
				realized_end_balance = EXCLUDED.realized_end_balance,
				previous_month_end_balance = EXCLUDED.previous_month_end_balance,
				pending_amortizations_count = EXCLUDED.pending_amortizations_count,
				pending_amortizations_amount = EXCLUDED.pending_amortizations_amount,
				deviation_highlights = EXCLUDED.deviation_highlights,
				is_completed = EXCLUDED.is_completed,
				completed_at = EXCLUDED.completed_at,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE monthly_reviews.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.Year, item.Month, item.PlannedIncome, item.RealizedIncome, item.PlannedExpense, item.RealizedExpense,
			item.PlannedEndBalance, item.RealizedEndBalance, item.PreviousMonthEndBalance,
			item.PendingAmortizationsCount, item.PendingAmortizationsAmount, item.DeviationHighlights,
			item.IsCompleted, item.CompletedAt, item.Notes, createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	case "ml_behavioral_profiles":
		var item models.MLBehavioralProfile
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return false, err
		}
		item.ID = m.EntityID
		item.UserID = userID
		createdAt := effectiveTime
		if !item.CreatedAt.IsZero() {
			createdAt = item.CreatedAt.UTC()
		}
		query := `
			INSERT INTO ml_behavioral_profiles (
				id, user_id, last_trained_at, training_epoch_count, analyzed_transactions_count,
				inflation_sensitivity, anomaly_z_threshold, subscription_drift_threshold_percent,
				category_weight_adjustments, day_of_month_spend_curve, category_stats,
				useful_feedback_count, ignored_feedback_count, dismissed_feedback_count,
				created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (id) DO UPDATE SET
				last_trained_at = EXCLUDED.last_trained_at,
				training_epoch_count = EXCLUDED.training_epoch_count,
				analyzed_transactions_count = EXCLUDED.analyzed_transactions_count,
				inflation_sensitivity = EXCLUDED.inflation_sensitivity,
				anomaly_z_threshold = EXCLUDED.anomaly_z_threshold,
				subscription_drift_threshold_percent = EXCLUDED.subscription_drift_threshold_percent,
				category_weight_adjustments = EXCLUDED.category_weight_adjustments,
				day_of_month_spend_curve = EXCLUDED.day_of_month_spend_curve,
				category_stats = EXCLUDED.category_stats,
				useful_feedback_count = EXCLUDED.useful_feedback_count,
				ignored_feedback_count = EXCLUDED.ignored_feedback_count,
				dismissed_feedback_count = EXCLUDED.dismissed_feedback_count,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE ml_behavioral_profiles.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.LastTrainedAt, item.TrainingEpochCount, item.AnalyzedTransactionsCount,
			item.InflationSensitivity, item.AnomalyZThreshold, item.SubscriptionDriftThresholdPercent,
			item.CategoryWeightAdjustments, item.DayOfMonthSpendCurve, item.CategoryStats,
			item.UsefulFeedbackCount, item.IgnoredFeedbackCount, item.DismissedFeedbackCount,
			createdAt, effectiveTime,
		)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil

	default:
		return false, nil
	}
}

// fetchEntityData retrieves the JSON data representation of an entity
func (s *SyncService) fetchEntityData(ctx context.Context, userID uuid.UUID, entityName string, entityID uuid.UUID) (map[string]interface{}, error) {
	var jsonStr string
	query := fmt.Sprintf("SELECT row_to_json(t) FROM (SELECT * FROM %s WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL) t", entityName)
	err := s.db.Pool.QueryRow(ctx, query, entityID, userID).Scan(&jsonStr)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// ResetUserData purges financial and sync data for the given user, either completely or for specific targets
func (s *SyncService) ResetUserData(ctx context.Context, userID uuid.UUID, targets []string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	targetSet := make(map[string]bool)
	for _, t := range targets {
		targetSet[strings.ToLower(strings.TrimSpace(t))] = true
	}

	resetAll := len(targetSet) == 0

	var tablesToDelete []string
	if resetAll {
		tablesToDelete = []string{
			"sync_changelog",
			"amortization_plans",
			"amortization_records",
			"investment_movements",
			"financial_transactions",
			"credit_card_invoices",
			"credit_cards",
			"credit_contracts",
			"debt_items",
			"investment_items",
			"category_quota_rules",
			"category_provisions",
			"sinking_funds",
			"what_if_scenarios",
			"financial_insights",
			"monthly_reviews",
			"ml_behavioral_profiles",
			"categories",
			"bank_accounts",
			"asset_files",
			"chat_messages",
		}
	} else {
		// Dependent children are ordered before parent foreign keys
		if targetSet["transactions"] || targetSet["financial_transactions"] {
			tablesToDelete = append(tablesToDelete, "financial_transactions")
		}
		if targetSet["investments"] || targetSet["investment_items"] {
			tablesToDelete = append(tablesToDelete, "investment_movements", "investment_items")
		}
		if targetSet["loans"] || targetSet["financings"] || targetSet["credit_contracts"] {
			if targetSet["loans"] && !targetSet["financings"] {
				_, _ = tx.Exec(ctx, "DELETE FROM amortization_plans WHERE contract_id IN (SELECT id FROM credit_contracts WHERE user_id = $1 AND kind = 'loan')", userID)
				_, _ = tx.Exec(ctx, "DELETE FROM amortization_records WHERE contract_id IN (SELECT id FROM credit_contracts WHERE user_id = $1 AND kind = 'loan')", userID)
				_, _ = tx.Exec(ctx, "DELETE FROM credit_contracts WHERE user_id = $1 AND kind = 'loan'", userID)
			} else if targetSet["financings"] && !targetSet["loans"] {
				_, _ = tx.Exec(ctx, "DELETE FROM amortization_plans WHERE contract_id IN (SELECT id FROM credit_contracts WHERE user_id = $1 AND kind = 'financing')", userID)
				_, _ = tx.Exec(ctx, "DELETE FROM amortization_records WHERE contract_id IN (SELECT id FROM credit_contracts WHERE user_id = $1 AND kind = 'financing')", userID)
				_, _ = tx.Exec(ctx, "DELETE FROM credit_contracts WHERE user_id = $1 AND kind = 'financing'", userID)
			} else {
				tablesToDelete = append(tablesToDelete, "amortization_plans", "amortization_records", "credit_contracts")
			}
		}
		if targetSet["debts"] || targetSet["debt_items"] {
			tablesToDelete = append(tablesToDelete, "debt_items")
		}
		if targetSet["categories"] {
			tablesToDelete = append(tablesToDelete, "category_quota_rules", "category_provisions", "categories")
		}
		if targetSet["creditcards"] || targetSet["credit_cards"] || targetSet["credit_card_invoices"] {
			tablesToDelete = append(tablesToDelete, "credit_card_invoices", "credit_cards")
		}
		if targetSet["accounts"] || targetSet["bank_accounts"] {
			tablesToDelete = append(tablesToDelete, "bank_accounts")
		}
		if targetSet["aimodelsandchat"] || targetSet["chat"] || targetSet["chat_messages"] {
			tablesToDelete = append(tablesToDelete, "chat_messages")
		}
		if targetSet["sinkingfunds"] || targetSet["sinking_funds"] {
			tablesToDelete = append(tablesToDelete, "sinking_funds")
		}
		if targetSet["whatifscenarios"] || targetSet["what_if_scenarios"] {
			tablesToDelete = append(tablesToDelete, "what_if_scenarios")
		}
		if targetSet["insights"] || targetSet["financial_insights"] {
			tablesToDelete = append(tablesToDelete, "financial_insights", "monthly_reviews", "ml_behavioral_profiles")
		}
	}

	for _, table := range tablesToDelete {
		_, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE user_id = $1", table), userID)
		if err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}

	isWipingAll := resetAll || (targetSet["transactions"] && targetSet["accounts"] && targetSet["categories"])
	if isWipingAll {
		_, _ = tx.Exec(ctx, "DELETE FROM sync_changelog WHERE user_id = $1", userID)
	} else {
		for _, table := range tablesToDelete {
			_, _ = tx.Exec(ctx, "DELETE FROM sync_changelog WHERE user_id = $1 AND entity_name = $2", userID, table)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.invalidateHeadCache(userID)
	return nil
}

// CreateTransaction creates a financial transaction directly via REST API and records in sync_changelog
func (s *SyncService) CreateTransaction(ctx context.Context, userID uuid.UUID, tx models.FinancialTransaction) (*models.FinancialTransaction, error) {
	if tx.ID == uuid.Nil {
		tx.ID = uuid.New()
	}
	tx.UserID = userID
	now := time.Now().UTC()
	if tx.Date.IsZero() {
		tx.Date = now
	}
	if tx.RecurrenceMode == "" {
		tx.RecurrenceMode = "single"
	}
	if tx.StatementOccurrenceIndex == 0 {
		tx.StatementOccurrenceIndex = 1
	}
	tx.CreatedAt = now
	tx.UpdatedAt = now

	conn, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Rollback(ctx)

	accID, err := s.getOrFallbackAccount(ctx, conn, userID, tx.AccountID)
	if err != nil {
		return nil, fmt.Errorf("account validation failed: %w", err)
	}
	tx.AccountID = accID

	catID, err := s.getOrFallbackCategory(ctx, conn, userID, tx.CategoryID, tx.Type)
	if err != nil {
		return nil, fmt.Errorf("category validation failed: %w", err)
	}
	tx.CategoryID = catID

	query := `
		INSERT INTO financial_transactions (
			id, user_id, account_id, category_id, name, details, amount, date, type, is_paid,
			attachment_path, attachment_name, ignore_in_consolidated, recurrence_mode, recurrence_frequency,
			is_canceled_recurring_series, series_group_id, installment_index, installment_total,
			raw_statement_line, statement_occurrence_index, linked_contract_id, linked_investment_id,
			linked_debt_id, linked_quota_id, linked_quota_units, linked_quota_year_month_key,
			transaction_origin, linked_entity_id,
			base_installment_registered_amount, projected_installment_correction_amount, confirmed_real_interest_correction_amount,
			credit_card_id, credit_card_invoice_year_month_key, credit_card_invoice_id,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37)
	`
	_, err = conn.Exec(ctx, query,
		tx.ID, tx.UserID, tx.AccountID, tx.CategoryID, tx.Name, tx.Details, tx.Amount, tx.Date, tx.Type, tx.IsPaid,
		tx.AttachmentPath, tx.AttachmentName, tx.IgnoreInConsolidated, tx.RecurrenceMode, tx.RecurrenceFrequency,
		tx.IsCanceledRecurringSeries, tx.SeriesGroupID, tx.InstallmentIndex, tx.InstallmentTotal,
		tx.RawStatementLine, tx.StatementOccurrenceIndex, tx.LinkedContractID, tx.LinkedInvestmentID,
		tx.LinkedDebtID, tx.LinkedQuotaID, tx.LinkedQuotaUnits, tx.LinkedQuotaYearMonthKey,
		tx.TransactionOrigin, tx.LinkedEntityID,
		tx.BaseInstallmentRegisteredAmount, tx.ProjectedInstallmentCorrectionAmount, tx.ConfirmedRealInterestCorrectionAmount,
		tx.CreditCardID, tx.CreditCardInvoiceYearMonthKey, tx.CreditCardInvoiceID,
		tx.CreatedAt, tx.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert transaction: %w", err)
	}

	// Insert into sync_changelog with client_id = "api"
	_, err = conn.Exec(ctx, `
		INSERT INTO sync_changelog (user_id, client_id, entity_name, entity_id, operation, mutation_id, created_at)
		VALUES ($1, 'api', 'financial_transactions', $2, 'INSERT', $3, $4)
	`, userID, tx.ID, uuid.New(), now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert changelog: %w", err)
	}

	if err := conn.Commit(ctx); err != nil {
		return nil, err
	}
	s.invalidateHeadCache(userID)

	return &tx, nil
}

// DeleteTransaction soft-deletes a transaction and records in sync_changelog
func (s *SyncService) DeleteTransaction(ctx context.Context, userID uuid.UUID, txID uuid.UUID) error {
	conn, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer conn.Rollback(ctx)

	now := time.Now().UTC()
	cmd, err := conn.Exec(ctx, `
		UPDATE financial_transactions
		SET deleted_at = $1, updated_at = $1
		WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL
	`, now, txID, userID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("transação não encontrada")
	}

	// Insert into sync_changelog with client_id = "api"
	_, err = conn.Exec(ctx, `
		INSERT INTO sync_changelog (user_id, client_id, entity_name, entity_id, operation, mutation_id, created_at)
		VALUES ($1, 'api', 'financial_transactions', $2, 'DELETE', $3, $4)
	`, userID, txID, uuid.New(), now)
	if err != nil {
		return fmt.Errorf("failed to insert changelog: %w", err)
	}

	if err := conn.Commit(ctx); err != nil {
		return err
	}
	s.invalidateHeadCache(userID)
	return nil
}

// ListTransactions returns all active transactions for the user
func (s *SyncService) ListTransactions(ctx context.Context, userID uuid.UUID) ([]models.FinancialTransaction, error) {
	query := `
		SELECT id, user_id, account_id, category_id, name, details, amount, date, type, is_paid,
			attachment_path, attachment_name, ignore_in_consolidated, recurrence_mode, recurrence_frequency,
			is_canceled_recurring_series, series_group_id, installment_index, installment_total,
			raw_statement_line, statement_occurrence_index, linked_contract_id, linked_investment_id,
			linked_debt_id, linked_quota_id, linked_quota_units, linked_quota_year_month_key,
			transaction_origin, linked_entity_id,
			base_installment_registered_amount, projected_installment_correction_amount, confirmed_real_interest_correction_amount,
			credit_card_id, credit_card_invoice_year_month_key, credit_card_invoice_id,
			created_at, updated_at
		FROM financial_transactions
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY date DESC
	`
	rows, err := s.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.FinancialTransaction
	for rows.Next() {
		var tx models.FinancialTransaction
		err := rows.Scan(
			&tx.ID, &tx.UserID, &tx.AccountID, &tx.CategoryID, &tx.Name, &tx.Details, &tx.Amount, &tx.Date, &tx.Type, &tx.IsPaid,
			&tx.AttachmentPath, &tx.AttachmentName, &tx.IgnoreInConsolidated, &tx.RecurrenceMode, &tx.RecurrenceFrequency,
			&tx.IsCanceledRecurringSeries, &tx.SeriesGroupID, &tx.InstallmentIndex, &tx.InstallmentTotal,
			&tx.RawStatementLine, &tx.StatementOccurrenceIndex, &tx.LinkedContractID, &tx.LinkedInvestmentID,
			&tx.LinkedDebtID, &tx.LinkedQuotaID, &tx.LinkedQuotaUnits, &tx.LinkedQuotaYearMonthKey,
			&tx.TransactionOrigin, &tx.LinkedEntityID,
			&tx.BaseInstallmentRegisteredAmount, &tx.ProjectedInstallmentCorrectionAmount, &tx.ConfirmedRealInterestCorrectionAmount,
			&tx.CreditCardID, &tx.CreditCardInvoiceYearMonthKey, &tx.CreditCardInvoiceID,
			&tx.CreatedAt, &tx.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, tx)
	}
	return list, nil
}

// ListAccounts returns all active bank accounts for the user
func (s *SyncService) ListAccounts(ctx context.Context, userID uuid.UUID) ([]models.BankAccount, error) {
	query := `
		SELECT id, user_id, name, bank_preset_id, custom_image_path, icon_symbol, color_hex, initial_balance, agency, account_number, display_order, created_at, updated_at
		FROM bank_accounts
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY display_order ASC, name ASC
	`
	rows, err := s.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.BankAccount
	for rows.Next() {
		var acc models.BankAccount
		err := rows.Scan(
			&acc.ID, &acc.UserID, &acc.Name, &acc.BankPresetID, &acc.CustomImagePath, &acc.IconSymbol,
			&acc.ColorHex, &acc.InitialBalance, &acc.Agency, &acc.AccountNumber, &acc.DisplayOrder, &acc.CreatedAt, &acc.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, acc)
	}
	return list, nil
}

// ListCategories returns all active categories for the user
func (s *SyncService) ListCategories(ctx context.Context, userID uuid.UUID) ([]models.Category, error) {
	query := `
		SELECT id, user_id, name, type, icon_symbol, color_hex, keywords, is_system_default, display_order, created_at, updated_at
		FROM categories
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY display_order ASC, name ASC
	`
	rows, err := s.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Category
	for rows.Next() {
		var cat models.Category
		err := rows.Scan(
			&cat.ID, &cat.UserID, &cat.Name, &cat.Type, &cat.IconSymbol, &cat.ColorHex,
			&cat.Keywords, &cat.IsSystemDefault, &cat.DisplayOrder, &cat.CreatedAt, &cat.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, cat)
	}
	return list, nil
}
