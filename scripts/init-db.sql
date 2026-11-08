CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";

-- 1. Usuários
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email CITEXT UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(150) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 2. Changelog Global de Sincronização
CREATE TABLE IF NOT EXISTS sync_changelog (
    revision BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id VARCHAR(100) NOT NULL,
    entity_name VARCHAR(50) NOT NULL,
    entity_id UUID NOT NULL,
    operation VARCHAR(10) NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    mutation_id UUID UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sync_changelog_user_rev ON sync_changelog(user_id, revision);

-- 3. Assets Binários
CREATE TABLE IF NOT EXISTS asset_files (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256_hash VARCHAR(64) NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_asset_files_user ON asset_files(user_id) WHERE deleted_at IS NULL;

-- 4. Contas Bancárias
CREATE TABLE IF NOT EXISTS bank_accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    bank_preset_id VARCHAR(50),
    custom_image_path VARCHAR(500),
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'building.columns.fill',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#EC7000',
    initial_balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    agency VARCHAR(20) NOT NULL DEFAULT '',
    account_number VARCHAR(30) NOT NULL DEFAULT '',
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_bank_accounts_user ON bank_accounts(user_id) WHERE deleted_at IS NULL;

-- 5. Categorias
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('expense', 'income')),
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'tag.fill',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#6366F1',
    keywords TEXT[] NOT NULL DEFAULT '{}',
    is_system_default BOOLEAN NOT NULL DEFAULT FALSE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_categories_user_type ON categories(user_id, type) WHERE deleted_at IS NULL;

-- 6. Transações Financeiras
CREATE TABLE IF NOT EXISTS financial_transactions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    amount NUMERIC(15, 2) NOT NULL CHECK (amount >= 0),
    date TIMESTAMPTZ NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('expense', 'income')),
    is_paid BOOLEAN NOT NULL DEFAULT TRUE,
    attachment_path VARCHAR(500),
    attachment_name VARCHAR(255),
    ignore_in_consolidated BOOLEAN NOT NULL DEFAULT FALSE,
    recurrence_mode VARCHAR(20) NOT NULL DEFAULT 'single' 
        CHECK (recurrence_mode IN ('single', 'installment', 'repeated', 'recurring')),
    recurrence_frequency VARCHAR(20) 
        CHECK (recurrence_frequency IN ('weekly', 'biweekly', 'monthly', 'bimonthly', 'quarterly', 'semiannual', 'annual')),
    is_canceled_recurring_series BOOLEAN,
    series_group_id UUID,
    installment_index INT,
    installment_total INT,
    raw_statement_line TEXT,
    statement_occurrence_index INT NOT NULL DEFAULT 1,
    linked_contract_id UUID,
    linked_investment_id UUID,
    linked_debt_id UUID,
    linked_quota_id UUID,
    linked_quota_units INT,
    linked_quota_year_month_key INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_transactions_user_date ON financial_transactions(user_id, date) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_series ON financial_transactions(series_group_id) WHERE deleted_at IS NULL;

-- 7. Investimentos e Movimentações
CREATE TABLE IF NOT EXISTS investment_items (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    investment_account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    name VARCHAR(150) NOT NULL,
    group_name VARCHAR(150) NOT NULL,
    balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    show_in_consolidated BOOLEAN NOT NULL DEFAULT TRUE,
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'chart.line.uptrend.xyaxis',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#10B981',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS investment_movements (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    investment_item_id UUID NOT NULL REFERENCES investment_items(id) ON DELETE CASCADE,
    source_account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    investment_account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    type VARCHAR(15) NOT NULL CHECK (type IN ('deposit', 'withdrawal')),
    amount NUMERIC(15, 2) NOT NULL CHECK (amount >= 0),
    date TIMESTAMPTZ NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    generated_transaction_id UUID REFERENCES financial_transactions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 8. Contratos de Crédito e Amortizações
CREATE TABLE IF NOT EXISTS credit_contracts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    kind VARCHAR(15) NOT NULL CHECK (kind IN ('loan', 'financing')),
    name VARCHAR(150) NOT NULL,
    contract_number VARCHAR(100),
    loan_subtype VARCHAR(30) 
        CHECK (loan_subtype IN ('Consignado', 'Pessoal', 'Antecipação FGTS', 'Empresarial', 'Refinanciamento', 'Outros')),
    financing_subtype VARCHAR(20) 
        CHECK (financing_subtype IN ('Imóvel', 'Carro')),
    amortization_system VARCHAR(100) NOT NULL DEFAULT 'PRICE (Parcelas Fixas)',
    total_contracted_amount NUMERIC(15, 2) NOT NULL,
    outstanding_balance NUMERIC(15, 2),
    installment_amount NUMERIC(15, 2) NOT NULL,
    billing_day INT NOT NULL CHECK (billing_day BETWEEN 1 AND 31),
    total_installments INT NOT NULL CHECK (total_installments >= 1),
    monthly_interest_rate NUMERIC(8, 4) NOT NULL DEFAULT 0.0000,
    start_date TIMESTAMPTZ,
    past_installments_elapsed_at_creation INT NOT NULL DEFAULT 0,
    extra_amortized_total NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    is_settled BOOLEAN NOT NULL DEFAULT FALSE,
    settled_amount NUMERIC(15, 2),
    settled_date TIMESTAMPTZ,
    settled_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS amortization_records (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contract_id UUID NOT NULL REFERENCES credit_contracts(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    transaction_id UUID REFERENCES financial_transactions(id) ON DELETE SET NULL,
    date TIMESTAMPTZ NOT NULL,
    amount_paid NUMERIC(15, 2) NOT NULL,
    mode VARCHAR(100) NOT NULL,
    previous_installment_amount NUMERIC(15, 2) NOT NULL,
    new_installment_amount NUMERIC(15, 2) NOT NULL,
    previous_remaining_installments INT NOT NULL,
    new_remaining_installments INT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 9. Dívidas
CREATE TABLE IF NOT EXISTS debt_items (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(150) NOT NULL,
    custom_image_path VARCHAR(500),
    bank_preset_id VARCHAR(50),
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'creditcard.trianglebadge.exclamationmark',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#EF4444',
    original_amount NUMERIC(15, 2) NOT NULL,
    contracted_date TIMESTAMPTZ,
    notes TEXT NOT NULL DEFAULT '',
    is_paid BOOLEAN NOT NULL DEFAULT FALSE,
    negotiated_amount NUMERIC(15, 2),
    paid_date TIMESTAMPTZ,
    paid_from_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL,
    payment_transaction_id UUID REFERENCES financial_transactions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 10. Provisões por Categoria e Cotas de Itens Específicos
CREATE TABLE IF NOT EXISTS category_provisions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    year INT NOT NULL,
    month INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    estimated_amount NUMERIC(15, 2) NOT NULL CHECK (estimated_amount >= 0),
    repeat_for_future_months BOOLEAN NOT NULL DEFAULT TRUE,
    monthly_overrides JSONB,
    excluded_year_month_keys INT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS category_quota_rules (
    id UUID PRIMARY KEY,
    series_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    name VARCHAR(150) NOT NULL,
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'shippingbox.fill',
    year INT NOT NULL,
    month INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    planned_quantity INT NOT NULL CHECK (planned_quantity >= 0),
    estimated_unit_price NUMERIC(15, 2) NOT NULL CHECK (estimated_unit_price >= 0),
    repeat_for_future_months BOOLEAN NOT NULL DEFAULT TRUE,
    monthly_overrides JSONB,
    excluded_year_month_keys INT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 11. Modelos de IA e Tarefas de Download no Backend (Escopo Global / Gestão Admin)
CREATE TABLE IF NOT EXISTS ai_models (
    id VARCHAR(150) PRIMARY KEY,
    display_name VARCHAR(150) NOT NULL,
    repo_id VARCHAR(150) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    local_path VARCHAR(500) NOT NULL,
    size_bytes BIGINT NOT NULL,
    quantization VARCHAR(50) NOT NULL DEFAULT 'Q4_K_M',
    is_global_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_download_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    repo_id VARCHAR(150) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('PENDING', 'DOWNLOADING', 'VERIFYING', 'READY', 'FAILED', 'CANCELED')),
    progress_percentage NUMERIC(5, 2) NOT NULL DEFAULT 0.00,
    downloaded_bytes BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    speed_bytes_sec BIGINT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ai_download_status ON ai_download_tasks(status);

CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model_used VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
