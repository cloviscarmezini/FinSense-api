package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"finsense-api/internal/database"
	"finsense-api/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SyncService struct {
	db *database.DB
}

func NewSyncService(db *database.DB) *SyncService {
	return &SyncService{db: db}
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
		case "financial_transactions":
			return 1
		case "category_quota_rules", "category_provisions", "credit_contracts", "investment_items", "debt_items":
			return 2
		case "categories":
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
		case "categories":
			return 2
		case "credit_contracts", "investment_items", "debt_items":
			return 3
		case "category_provisions", "category_quota_rules":
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

func initEmptyDeltas() map[string]interface{} {
	return map[string]interface{}{
		"bank_accounts":          []interface{}{},
		"bankAccounts":           []interface{}{},
		"categories":             []interface{}{},
		"financial_transactions": []interface{}{},
		"financialTransactions":  []interface{}{},
		"investment_items":       []interface{}{},
		"investmentItems":        []interface{}{},
		"debt_items":             []interface{}{},
		"debtItems":              []interface{}{},
		"credit_contracts":       []interface{}{},
		"creditContracts":        []interface{}{},
		"category_provisions":    []interface{}{},
		"categoryProvisions":     []interface{}{},
		"category_quota_rules":   []interface{}{},
		"categoryQuotaRules":     []interface{}{},
	}
}

func appendToDeltas(deltas map[string]interface{}, entityName string, data map[string]interface{}) {
	switch entityName {
	case "bank_accounts":
		deltas["bank_accounts"] = append(deltas["bank_accounts"].([]interface{}), data)
		deltas["bankAccounts"] = append(deltas["bankAccounts"].([]interface{}), data)
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
	case "category_provisions":
		deltas["category_provisions"] = append(deltas["category_provisions"].([]interface{}), data)
		deltas["categoryProvisions"] = append(deltas["categoryProvisions"].([]interface{}), data)
	case "category_quota_rules":
		deltas["category_quota_rules"] = append(deltas["category_quota_rules"].([]interface{}), data)
		deltas["categoryQuotaRules"] = append(deltas["categoryQuotaRules"].([]interface{}), data)
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
		"categories",
		"financial_transactions",
		"investment_items",
		"debt_items",
		"credit_contracts",
		"category_provisions",
		"category_quota_rules",
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
		"categories":             true,
		"financial_transactions": true,
		"debt_items":             true,
		"investment_items":       true,
		"credit_contracts":       true,
		"category_provisions":    true,
		"category_quota_rules":   true,
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

		accID, err := s.getOrFallbackAccount(ctx, tx, userID, item.AccountID)
		if err != nil {
			return false, fmt.Errorf("account validation failed: %w", err)
		}
		item.AccountID = accID

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
			_ = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM category_quota_rules WHERE id = $1 AND user_id = $2)", *item.LinkedQuotaID, userID).Scan(&exists)
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
				linked_debt_id, linked_quota_id, linked_quota_units, linked_quota_year_month_key, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
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
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE financial_transactions.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.AccountID, item.CategoryID, item.Name, item.Details, item.Amount, item.Date, item.Type, item.IsPaid,
			item.AttachmentPath, item.AttachmentName, item.IgnoreInConsolidated, item.RecurrenceMode, item.RecurrenceFrequency,
			item.IsCanceledRecurringSeries, item.SeriesGroupID, item.InstallmentIndex, item.InstallmentTotal,
			item.RawStatementLine, item.StatementOccurrenceIndex, item.LinkedContractID, item.LinkedInvestmentID,
			item.LinkedDebtID, item.LinkedQuotaID, item.LinkedQuotaUnits, item.LinkedQuotaYearMonthKey, createdAt, effectiveTime,
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

		query := `
			INSERT INTO investment_items (id, user_id, investment_account_id, name, group_name, balance, show_in_consolidated, icon_symbol, color_hex, notes, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (id) DO UPDATE SET
				investment_account_id = EXCLUDED.investment_account_id,
				name = EXCLUDED.name,
				group_name = EXCLUDED.group_name,
				balance = EXCLUDED.balance,
				show_in_consolidated = EXCLUDED.show_in_consolidated,
				icon_symbol = EXCLUDED.icon_symbol,
				color_hex = EXCLUDED.color_hex,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE investment_items.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.UserID, item.InvestmentAccountID, item.Name, item.GroupName, item.Balance, item.ShowInConsolidated, item.IconSymbol, item.ColorHex, item.Notes, createdAt, effectiveTime)
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

		query := `
			INSERT INTO credit_contracts (
				id, user_id, account_id, category_id, kind, name, contract_number, loan_subtype, financing_subtype,
				amortization_system, total_contracted_amount, outstanding_balance, installment_amount, billing_day,
				total_installments, monthly_interest_rate, start_date, past_installments_elapsed_at_creation,
				extra_amortized_total, is_settled, settled_amount, settled_date, settled_account_id, notes, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
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
				monthly_interest_rate = EXCLUDED.monthly_interest_rate,
				start_date = EXCLUDED.start_date,
				past_installments_elapsed_at_creation = EXCLUDED.past_installments_elapsed_at_creation,
				extra_amortized_total = EXCLUDED.extra_amortized_total,
				is_settled = EXCLUDED.is_settled,
				settled_amount = EXCLUDED.settled_amount,
				settled_date = EXCLUDED.settled_date,
				settled_account_id = EXCLUDED.settled_account_id,
				notes = EXCLUDED.notes,
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE credit_contracts.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.AccountID, item.CategoryID, item.Kind, item.Name, item.ContractNumber, item.LoanSubtype, item.FinancingSubtype,
			item.AmortizationSystem, item.TotalContractedAmount, item.OutstandingBalance, item.InstallmentAmount, item.BillingDay,
			item.TotalInstallments, item.MonthlyInterestRate, item.StartDate, item.PastInstallmentsElapsedAtCreation,
			item.ExtraAmortizedTotal, item.IsSettled, item.SettledAmount, item.SettledDate, item.SettledAccountID, item.Notes, createdAt, effectiveTime,
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
		catID, err := s.getOrFallbackCategory(ctx, tx, userID, item.CategoryID, "expense")
		if err != nil {
			return false, fmt.Errorf("quota category validation failed: %w", err)
		}
		item.CategoryID = catID

		query := `
			INSERT INTO category_quota_rules (id, series_id, user_id, category_id, name, icon_symbol, year, month, planned_quantity, estimated_unit_price, repeat_for_future_months, monthly_overrides, excluded_year_month_keys, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
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
				updated_at = EXCLUDED.updated_at,
				deleted_at = NULL
			WHERE category_quota_rules.updated_at <= EXCLUDED.updated_at;
		`
		tag, err := tx.Exec(ctx, query, item.ID, item.SeriesID, item.UserID, item.CategoryID, item.Name, item.IconSymbol, item.Year, item.Month, item.PlannedQuantity, item.EstimatedUnitPrice, item.RepeatForFutureMonths, item.MonthlyOverrides, item.ExcludedYearMonthKeys, createdAt, effectiveTime)
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

// ResetUserData purges all financial and sync data for the given user
func (s *SyncService) ResetUserData(ctx context.Context, userID uuid.UUID) error {
	tables := []string{
		"sync_changelog",
		"amortization_records",
		"investment_movements",
		"financial_transactions",
		"credit_contracts",
		"debt_items",
		"investment_items",
		"category_quota_rules",
		"category_provisions",
		"categories",
		"bank_accounts",
		"asset_files",
		"chat_messages",
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, table := range tables {
		_, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE user_id = $1", table), userID)
		if err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}

	return tx.Commit(ctx)
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
			linked_debt_id, linked_quota_id, linked_quota_units, linked_quota_year_month_key, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
	`
	_, err = conn.Exec(ctx, query,
		tx.ID, tx.UserID, tx.AccountID, tx.CategoryID, tx.Name, tx.Details, tx.Amount, tx.Date, tx.Type, tx.IsPaid,
		tx.AttachmentPath, tx.AttachmentName, tx.IgnoreInConsolidated, tx.RecurrenceMode, tx.RecurrenceFrequency,
		tx.IsCanceledRecurringSeries, tx.SeriesGroupID, tx.InstallmentIndex, tx.InstallmentTotal,
		tx.RawStatementLine, tx.StatementOccurrenceIndex, tx.LinkedContractID, tx.LinkedInvestmentID,
		tx.LinkedDebtID, tx.LinkedQuotaID, tx.LinkedQuotaUnits, tx.LinkedQuotaYearMonthKey, tx.CreatedAt, tx.UpdatedAt,
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

	return conn.Commit(ctx)
}

// ListTransactions returns all active transactions for the user
func (s *SyncService) ListTransactions(ctx context.Context, userID uuid.UUID) ([]models.FinancialTransaction, error) {
	query := `
		SELECT id, user_id, account_id, category_id, name, details, amount, date, type, is_paid,
			attachment_path, attachment_name, ignore_in_consolidated, recurrence_mode, recurrence_frequency,
			is_canceled_recurring_series, series_group_id, installment_index, installment_total,
			raw_statement_line, statement_occurrence_index, linked_contract_id, linked_investment_id,
			linked_debt_id, linked_quota_id, linked_quota_units, linked_quota_year_month_key, created_at, updated_at
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
			&tx.LinkedDebtID, &tx.LinkedQuotaID, &tx.LinkedQuotaUnits, &tx.LinkedQuotaYearMonthKey, &tx.CreatedAt, &tx.UpdatedAt,
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
