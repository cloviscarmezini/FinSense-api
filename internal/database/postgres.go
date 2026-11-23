package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"finsense-api/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(cfg *config.Config) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.DBMaxOpenConns)
	poolConfig.MinConns = int32(cfg.DBMaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.DBConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		log.Printf("[DB WARNING] Failed to ping database on startup: %v. Database might still be initializing.", err)
	} else {
		log.Println("[DB] Successfully connected to PostgreSQL.")
		// Run idempotent schema migrations
		if err := runMigrations(ctx, pool); err != nil {
			log.Printf("[DB WARNING] Failed to run schema migrations: %v", err)
		}
	}

	return &DB{Pool: pool}, nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrations := []string{
		`ALTER TABLE category_quota_rules ADD COLUMN IF NOT EXISTS quota_type VARCHAR(50) NOT NULL DEFAULT 'expense';`,
		`ALTER TABLE category_quota_rules ADD COLUMN IF NOT EXISTS linked_investment_id UUID REFERENCES investment_items(id) ON DELETE SET NULL;`,
		`ALTER TABLE category_quota_rules ALTER COLUMN category_id DROP NOT NULL;`,
		`ALTER TABLE financial_transactions ADD COLUMN IF NOT EXISTS base_installment_registered_amount NUMERIC(15, 2);`,
		`ALTER TABLE financial_transactions ADD COLUMN IF NOT EXISTS projected_installment_correction_amount NUMERIC(15, 2);`,
		`ALTER TABLE financial_transactions ADD COLUMN IF NOT EXISTS confirmed_real_interest_correction_amount NUMERIC(15, 2);`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS bank_due_day INT;`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS reference_date TIMESTAMPTZ;`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS maturity_date TIMESTAMPTZ;`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS estimated_monthly_tr NUMERIC(8, 4);`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS last_fgts_usage_date TIMESTAMPTZ;`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS scheduled_installments JSONB;`,
		`ALTER TABLE credit_contracts ADD COLUMN IF NOT EXISTS average_monthly_installment_growth NUMERIC(15, 6);`,
		`CREATE TABLE IF NOT EXISTS amortization_plans (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			contract_id UUID NOT NULL REFERENCES credit_contracts(id) ON DELETE CASCADE,
			generation_mode VARCHAR(50) NOT NULL DEFAULT 'Manual',
			billing_day INT NOT NULL DEFAULT 26,
			income_commitment_percentage NUMERIC(6, 2) NOT NULL DEFAULT 30.00,
			default_recalculation_mode VARCHAR(100) NOT NULL DEFAULT 'Reduzir Prazo (Quantidade de Parcelas)',
			estimated_monthly_tr NUMERIC(8, 4) NOT NULL DEFAULT 0.0000,
			items JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_amortization_plans_user_contract ON amortization_plans(user_id, contract_id) WHERE deleted_at IS NULL;`,
		`ALTER TABLE financial_transactions DROP CONSTRAINT IF EXISTS financial_transactions_account_id_fkey;`,
		`ALTER TABLE financial_transactions ADD COLUMN IF NOT EXISTS credit_card_id UUID;`,
		`ALTER TABLE financial_transactions ADD COLUMN IF NOT EXISTS credit_card_invoice_year_month_key INT;`,
		`ALTER TABLE financial_transactions ADD COLUMN IF NOT EXISTS credit_card_invoice_id UUID;`,
		`CREATE INDEX IF NOT EXISTS idx_transactions_credit_card ON financial_transactions(user_id, credit_card_id) WHERE deleted_at IS NULL AND credit_card_id IS NOT NULL;`,
		`CREATE TABLE IF NOT EXISTS credit_cards (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(120) NOT NULL,
			total_limit NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			closing_day INT NOT NULL DEFAULT 25 CHECK (closing_day BETWEEN 1 AND 31),
			best_purchase_day INT NOT NULL DEFAULT 26 CHECK (best_purchase_day BETWEEN 1 AND 31),
			due_day INT NOT NULL DEFAULT 5 CHECK (due_day BETWEEN 1 AND 31),
			default_payment_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL,
			brand VARCHAR(40) NOT NULL DEFAULT 'Mastercard',
			bank_preset_id VARCHAR(50),
			custom_image_path VARCHAR(500),
			icon_symbol VARCHAR(100) NOT NULL DEFAULT 'creditcard.fill',
			color_hex VARCHAR(9) NOT NULL DEFAULT '#820AD1',
			last_four_digits VARCHAR(10) NOT NULL DEFAULT '',
			display_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_credit_cards_user ON credit_cards(user_id) WHERE deleted_at IS NULL;`,
		`CREATE TABLE IF NOT EXISTS credit_card_invoices (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			credit_card_id UUID NOT NULL REFERENCES credit_cards(id) ON DELETE CASCADE,
			year INT NOT NULL,
			month INT NOT NULL CHECK (month BETWEEN 1 AND 12),
			closing_date TIMESTAMPTZ NOT NULL,
			due_date TIMESTAMPTZ NOT NULL,
			is_paid BOOLEAN NOT NULL DEFAULT FALSE,
			paid_amount NUMERIC(15, 2),
			paid_date TIMESTAMPTZ,
			paid_from_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL,
			payment_transaction_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_credit_card_invoices_user_card ON credit_card_invoices(user_id, credit_card_id, year, month) WHERE deleted_at IS NULL;`,
		`CREATE TABLE IF NOT EXISTS sinking_funds (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(150) NOT NULL,
			category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
			icon_symbol VARCHAR(100) NOT NULL DEFAULT 'tray.and.arrow.down.fill',
			color_hex VARCHAR(9) NOT NULL DEFAULT '#0EA5E9',
			target_amount NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			recurrence_months INT NOT NULL DEFAULT 12,
			due_month INT NOT NULL DEFAULT 1,
			due_year INT NOT NULL DEFAULT 2027,
			accumulated_reserve NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sinking_funds_user ON sinking_funds(user_id) WHERE deleted_at IS NULL;`,
		`CREATE TABLE IF NOT EXISTS what_if_scenarios (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(150) NOT NULL,
			variable_income_reduction_percent NUMERIC(6, 2) NOT NULL DEFAULT 0.00,
			unexpected_expense_amount NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			unexpected_expense_month_offset INT NOT NULL DEFAULT 0,
			benchmark_annual_rate_percent NUMERIC(6, 2) NOT NULL DEFAULT 11.25,
			net_yield_tax_rate_percent NUMERIC(6, 2) NOT NULL DEFAULT 15.00,
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE TABLE IF NOT EXISTS financial_insights (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			fingerprint VARCHAR(200) NOT NULL,
			kind VARCHAR(60) NOT NULL,
			severity VARCHAR(20) NOT NULL DEFAULT 'warning',
			title VARCHAR(255) NOT NULL,
			message TEXT NOT NULL,
			recommendation TEXT NOT NULL DEFAULT '',
			impact_amount NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			confidence_score NUMERIC(5, 4) NOT NULL DEFAULT 0.8000,
			year_month_key INT NOT NULL DEFAULT 202601,
			related_category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
			related_transaction_id UUID,
			feedback VARCHAR(20) NOT NULL DEFAULT 'pending',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_financial_insights_user ON financial_insights(user_id) WHERE deleted_at IS NULL;`,
		`CREATE TABLE IF NOT EXISTS monthly_reviews (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			year INT NOT NULL,
			month INT NOT NULL,
			planned_income NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			realized_income NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			planned_expense NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			realized_expense NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			planned_end_balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			realized_end_balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			previous_month_end_balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			pending_amortizations_count INT NOT NULL DEFAULT 0,
			pending_amortizations_amount NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
			deviation_highlights JSONB,
			is_completed BOOLEAN NOT NULL DEFAULT FALSE,
			completed_at TIMESTAMPTZ,
			notes TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
		`CREATE TABLE IF NOT EXISTS ml_behavioral_profiles (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			last_trained_at TIMESTAMPTZ,
			training_epoch_count INT NOT NULL DEFAULT 0,
			analyzed_transactions_count INT NOT NULL DEFAULT 0,
			inflation_sensitivity NUMERIC(6, 4) NOT NULL DEFAULT 1.0000,
			anomaly_z_threshold NUMERIC(6, 4) NOT NULL DEFAULT 2.2000,
			subscription_drift_threshold_percent NUMERIC(6, 4) NOT NULL DEFAULT 3.0000,
			category_weight_adjustments JSONB,
			day_of_month_spend_curve JSONB,
			category_stats JSONB,
			useful_feedback_count INT NOT NULL DEFAULT 0,
			ignored_feedback_count INT NOT NULL DEFAULT 0,
			dismissed_feedback_count INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		);`,
	}

	for _, stmt := range migrations {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			log.Printf("[DB MIGRATION NOTE] Statement execution note: %v", err)
		}
	}
	return nil
}

func (db *DB) Ping(ctx context.Context) error {
	if db.Pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return db.Pool.Ping(ctx)
}

func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
