package services

import (
	"context"
	"encoding/json"
	"fmt"
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

// PushPull handles processing client mutations and returning server deltas
func (s *SyncService) PushPull(ctx context.Context, userID uuid.UUID, req models.PushPullRequest) (*models.PushPullResponse, error) {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	appliedIDs := make([]uuid.UUID, 0, len(req.Mutations))

	for _, m := range req.Mutations {
		// Check idempotency
		var exists bool
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sync_changelog WHERE mutation_id = $1)", m.MutationID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("failed to check mutation idempotency: %w", err)
		}

		if exists {
			appliedIDs = append(appliedIDs, m.MutationID)
			continue
		}

		// Apply mutation based on entity type and operation
		if err := s.applyMutation(ctx, tx, userID, m); err != nil {
			// Log error but continue or return error
			return nil, fmt.Errorf("failed to apply mutation %s on entity %s: %w", m.MutationID, m.EntityName, err)
		}

		// Record in changelog
		_, err = tx.Exec(ctx, `
			INSERT INTO sync_changelog (user_id, client_id, entity_name, entity_id, operation, mutation_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, userID, req.ClientID, m.EntityName, m.EntityID, string(m.Operation), m.MutationID, time.Now().UTC())
		if err != nil {
			return nil, fmt.Errorf("failed to insert changelog: %w", err)
		}

		appliedIDs = append(appliedIDs, m.MutationID)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch server changes since since_revision
	serverChanges, latestRev, err := s.GetDelta(ctx, userID, req.ClientID, req.SinceRevision)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server changes: %w", err)
	}

	return &models.PushPullResponse{
		LatestRevision:     latestRev,
		AppliedMutationIDs: appliedIDs,
		ServerChanges:      serverChanges,
	}, nil
}

// GetDelta returns changelog records since since_revision
func (s *SyncService) GetDelta(ctx context.Context, userID uuid.UUID, clientID string, sinceRev int64) ([]models.Mutation, int64, error) {
	var currentLatest int64
	err := s.db.Pool.QueryRow(ctx, "SELECT COALESCE(MAX(revision), 0) FROM sync_changelog WHERE user_id = $1", userID).Scan(&currentLatest)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT revision, entity_name, entity_id, operation, mutation_id, created_at
		FROM sync_changelog
		WHERE user_id = $1 AND revision > $2 AND ($3 = '' OR client_id != $3)
		ORDER BY revision ASC
		LIMIT 1000
	`

	rows, err := s.db.Pool.Query(ctx, query, userID, sinceRev, clientID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var changes []models.Mutation
	for rows.Next() {
		var rev int64
		var entityName string
		var entityID uuid.UUID
		var opStr string
		var mutID uuid.UUID
		var createdAt time.Time

		if err := rows.Scan(&rev, &entityName, &entityID, &opStr, &mutID, &createdAt); err != nil {
			return nil, 0, err
		}

		// Retrieve entity data if not deleted
		entityData, _ := s.fetchEntityData(ctx, userID, entityName, entityID)

		changes = append(changes, models.Mutation{
			MutationID: mutID,
			EntityName: entityName,
			EntityID:   entityID,
			Operation:  models.OperationType(opStr),
			Data:       entityData,
			Timestamp:  createdAt,
		})
	}

	return changes, currentLatest, nil
}

// applyMutation executes insert/update/delete according to entity name
func (s *SyncService) applyMutation(ctx context.Context, tx pgx.Tx, userID uuid.UUID, m models.Mutation) error {
	now := time.Now().UTC()
	jsonBytes, _ := json.Marshal(m.Data)

	switch m.EntityName {
	case "bank_accounts":
		if m.Operation == models.OpDelete {
			_, err := tx.Exec(ctx, "UPDATE bank_accounts SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND user_id = $3", now, m.EntityID, userID)
			return err
		}
		var item models.BankAccount
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return err
		}
		item.ID = m.EntityID
		item.UserID = userID
		item.UpdatedAt = now

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
		_, err := tx.Exec(ctx, query, item.ID, item.UserID, item.Name, item.BankPresetID, item.CustomImagePath, item.IconSymbol, item.ColorHex, item.InitialBalance, item.Agency, item.AccountNumber, item.DisplayOrder, now, now)
		return err

	case "categories":
		if m.Operation == models.OpDelete {
			_, err := tx.Exec(ctx, "UPDATE categories SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND user_id = $3", now, m.EntityID, userID)
			return err
		}
		var item models.Category
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return err
		}
		item.ID = m.EntityID
		item.UserID = userID
		item.UpdatedAt = now

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
		_, err := tx.Exec(ctx, query, item.ID, item.UserID, item.Name, item.Type, item.IconSymbol, item.ColorHex, item.Keywords, item.IsSystemDefault, item.DisplayOrder, now, now)
		return err

	case "financial_transactions":
		if m.Operation == models.OpDelete {
			_, err := tx.Exec(ctx, "UPDATE financial_transactions SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND user_id = $3", now, m.EntityID, userID)
			return err
		}
		var item models.FinancialTransaction
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return err
		}
		item.ID = m.EntityID
		item.UserID = userID
		item.UpdatedAt = now

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
		_, err := tx.Exec(ctx, query,
			item.ID, item.UserID, item.AccountID, item.CategoryID, item.Name, item.Details, item.Amount, item.Date, item.Type, item.IsPaid,
			item.AttachmentPath, item.AttachmentName, item.IgnoreInConsolidated, item.RecurrenceMode, item.RecurrenceFrequency,
			item.IsCanceledRecurringSeries, item.SeriesGroupID, item.InstallmentIndex, item.InstallmentTotal,
			item.RawStatementLine, item.StatementOccurrenceIndex, item.LinkedContractID, item.LinkedInvestmentID,
			item.LinkedDebtID, item.LinkedQuotaID, item.LinkedQuotaUnits, item.LinkedQuotaYearMonthKey, now, now,
		)
		return err

	case "debt_items":
		if m.Operation == models.OpDelete {
			_, err := tx.Exec(ctx, "UPDATE debt_items SET deleted_at = $1, updated_at = $1 WHERE id = $2 AND user_id = $3", now, m.EntityID, userID)
			return err
		}
		var item models.DebtItem
		if err := json.Unmarshal(jsonBytes, &item); err != nil {
			return err
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
		_, err := tx.Exec(ctx, query, m.EntityID, userID, item.Name, item.CustomImagePath, item.BankPresetID, item.IconSymbol, item.ColorHex, item.OriginalAmount, item.ContractedDate, item.Notes, item.IsPaid, item.NegotiatedAmount, item.PaidDate, item.PaidFromAccountID, item.PaymentTransactionID, now, now)
		return err

	default:
		// Generic handling or ignore unknown
		return nil
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
