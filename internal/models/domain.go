package models

import (
	"time"

	"github.com/google/uuid"
)

// BankAccount represents a user bank account or credit card entity
type BankAccount struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	Name            string     `json:"name"`
	BankPresetID    *string    `json:"bank_preset_id,omitempty"`
	CustomImagePath *string    `json:"custom_image_path,omitempty"`
	IconSymbol      string     `json:"icon_symbol"`
	ColorHex        string     `json:"color_hex"`
	InitialBalance  float64    `json:"initial_balance"`
	Agency          string     `json:"agency"`
	AccountNumber   string     `json:"account_number"`
	DisplayOrder    int        `json:"display_order"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

// Category represents a classification for expenses or incomes
type Category struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	Name            string     `json:"name"`
	Type            string     `json:"type"` // "expense" or "income"
	IconSymbol      string     `json:"icon_symbol"`
	ColorHex        string     `json:"color_hex"`
	Keywords        []string   `json:"keywords"`
	IsSystemDefault bool       `json:"is_system_default"`
	DisplayOrder    int        `json:"display_order"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

// FinancialTransaction represents an income or expense movement
type FinancialTransaction struct {
	ID                         uuid.UUID  `json:"id"`
	UserID                     uuid.UUID  `json:"user_id"`
	AccountID                  uuid.UUID  `json:"account_id"`
	CategoryID                 uuid.UUID  `json:"category_id"`
	Name                       string     `json:"name"`
	Details                    string     `json:"details"`
	Amount                     float64    `json:"amount"`
	Date                       time.Time  `json:"date"`
	Type                       string     `json:"type"` // "expense" or "income"
	IsPaid                     bool       `json:"is_paid"`
	AttachmentPath             *string    `json:"attachment_path,omitempty"`
	AttachmentName             *string    `json:"attachment_name,omitempty"`
	IgnoreInConsolidated       bool       `json:"ignore_in_consolidated"`
	RecurrenceMode             string     `json:"recurrence_mode"` // single, installment, repeated, recurring
	RecurrenceFrequency        *string    `json:"recurrence_frequency,omitempty"`
	IsCanceledRecurringSeries  *bool      `json:"is_canceled_recurring_series,omitempty"`
	SeriesGroupID              *uuid.UUID `json:"series_group_id,omitempty"`
	InstallmentIndex           *int       `json:"installment_index,omitempty"`
	InstallmentTotal           *int       `json:"installment_total,omitempty"`
	RawStatementLine           *string    `json:"raw_statement_line,omitempty"`
	StatementOccurrenceIndex   int        `json:"statement_occurrence_index"`
	LinkedContractID           *uuid.UUID `json:"linked_contract_id,omitempty"`
	LinkedInvestmentID         *uuid.UUID `json:"linked_investment_id,omitempty"`
	LinkedDebtID               *uuid.UUID `json:"linked_debt_id,omitempty"`
	LinkedQuotaID              *uuid.UUID `json:"linked_quota_id,omitempty"`
	LinkedQuotaUnits           *int       `json:"linked_quota_units,omitempty"`
	LinkedQuotaYearMonthKey    *int       `json:"linked_quota_year_month_key,omitempty"`
	TransactionOrigin          *string    `json:"transaction_origin,omitempty"`
	LinkedEntityID             *uuid.UUID `json:"linked_entity_id,omitempty"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	DeletedAt                  *time.Time `json:"deleted_at,omitempty"`
}

// InvestmentItem represents an asset portfolio position
type InvestmentItem struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               uuid.UUID  `json:"user_id"`
	InvestmentAccountID  uuid.UUID  `json:"investment_account_id"`
	Name                 string     `json:"name"`
	GroupName            string     `json:"group_name"`
	Balance              float64    `json:"balance"`
	ShowInConsolidated   bool       `json:"show_in_consolidated"`
	IconSymbol           string     `json:"icon_symbol"`
	ColorHex             string     `json:"color_hex"`
	Notes                string      `json:"notes"`
	Movements            interface{} `json:"movements,omitempty"`
	CreatedAt            time.Time   `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
}

// InvestmentMovement represents a deposit or withdrawal in an investment
type InvestmentMovement struct {
	ID                     uuid.UUID  `json:"id"`
	UserID                 uuid.UUID  `json:"user_id"`
	InvestmentItemID       uuid.UUID  `json:"investment_item_id"`
	SourceAccountID        uuid.UUID  `json:"source_account_id"`
	InvestmentAccountID    uuid.UUID  `json:"investment_account_id"`
	Type                   string     `json:"type"` // "deposit" or "withdrawal"
	Amount                 float64    `json:"amount"`
	Date                   time.Time  `json:"date"`
	Notes                  string     `json:"notes"`
	GeneratedTransactionID *uuid.UUID `json:"generated_transaction_id,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	DeletedAt              *time.Time `json:"deleted_at,omitempty"`
}

// CreditContract represents a loan or financing contract
type CreditContract struct {
	ID                                uuid.UUID  `json:"id"`
	UserID                            uuid.UUID  `json:"user_id"`
	AccountID                         uuid.UUID  `json:"account_id"`
	CategoryID                        uuid.UUID  `json:"category_id"`
	Kind                              string     `json:"kind"` // "loan" or "financing"
	Name                              string     `json:"name"`
	ContractNumber                    *string    `json:"contract_number,omitempty"`
	LoanSubtype                       *string    `json:"loan_subtype,omitempty"`
	FinancingSubtype                  *string    `json:"financing_subtype,omitempty"`
	AmortizationSystem                string     `json:"amortization_system"`
	TotalContractedAmount             float64    `json:"total_contracted_amount"`
	OutstandingBalance                *float64   `json:"outstanding_balance,omitempty"`
	InstallmentAmount                 float64    `json:"installment_amount"`
	BillingDay                        int        `json:"billing_day"`
	TotalInstallments                 int         `json:"total_installments"`
	OriginalTotalInstallments         *int        `json:"original_total_installments,omitempty"`
	MonthlyInterestRate               float64     `json:"monthly_interest_rate"`
	AnnualNominalInterestRate         *float64    `json:"annual_nominal_interest_rate,omitempty"`
	IndexationRate                    *string     `json:"indexation_rate,omitempty"`
	AdminFee                          *float64    `json:"admin_fee,omitempty"`
	InsuranceFee                      *float64    `json:"insurance_fee,omitempty"`
	BaseInstallmentAmount             *float64    `json:"base_installment_amount,omitempty"`
	FinancingDetails                  interface{} `json:"financing_details,omitempty"`
	LoanDetails                       interface{} `json:"loan_details,omitempty"`
	AmortizationHistory               interface{} `json:"amortization_history,omitempty"`
	StartDate                         *time.Time  `json:"start_date,omitempty"`
	PastInstallmentsElapsedAtCreation int        `json:"past_installments_elapsed_at_creation"`
	ExtraAmortizedTotal               float64    `json:"extra_amortized_total"`
	IsSettled                         bool       `json:"is_settled"`
	SettledAmount                     *float64   `json:"settled_amount,omitempty"`
	SettledDate                       *time.Time `json:"settled_date,omitempty"`
	SettledAccountID                  *uuid.UUID `json:"settled_account_id,omitempty"`
	Notes                             string     `json:"notes"`
	CreatedAt                         time.Time  `json:"created_at"`
	UpdatedAt                         time.Time  `json:"updated_at"`
	DeletedAt                         *time.Time `json:"deleted_at,omitempty"`
}

// AmortizationRecord represents an extraordinary amortization
type AmortizationRecord struct {
	ID                           uuid.UUID  `json:"id"`
	UserID                       uuid.UUID  `json:"user_id"`
	ContractID                   uuid.UUID  `json:"contract_id"`
	AccountID                    uuid.UUID  `json:"account_id"`
	TransactionID                *uuid.UUID `json:"transaction_id,omitempty"`
	Date                         time.Time  `json:"date"`
	AmountPaid                   float64    `json:"amount_paid"`
	Mode                         string     `json:"mode"`
	PreviousInstallmentAmount    float64    `json:"previous_installment_amount"`
	NewInstallmentAmount         float64    `json:"new_installment_amount"`
	PreviousRemainingInstallments int       `json:"previous_remaining_installments"`
	NewRemainingInstallments      int       `json:"new_remaining_installments"`
	EffectiveAmortization         *float64  `json:"effective_amortization,omitempty"`
	PreviousBalance               *float64  `json:"previous_balance,omitempty"`
	Notes                         string    `json:"notes"`
	CreatedAt                    time.Time  `json:"created_at"`
	UpdatedAt                    time.Time  `json:"updated_at"`
	DeletedAt                    *time.Time `json:"deleted_at,omitempty"`
}

// DebtItem represents a liability/debt
type DebtItem struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               uuid.UUID  `json:"user_id"`
	Name                 string     `json:"name"`
	CustomImagePath      *string    `json:"custom_image_path,omitempty"`
	BankPresetID         *string    `json:"bank_preset_id,omitempty"`
	IconSymbol           string     `json:"icon_symbol"`
	ColorHex             string     `json:"color_hex"`
	OriginalAmount       float64    `json:"original_amount"`
	ContractedDate       *time.Time `json:"contracted_date,omitempty"`
	Notes                string     `json:"notes"`
	IsPaid               bool       `json:"is_paid"`
	NegotiatedAmount     *float64   `json:"negotiated_amount,omitempty"`
	PaidDate             *time.Time `json:"paid_date,omitempty"`
	PaidFromAccountID    *uuid.UUID `json:"paid_from_account_id,omitempty"`
	PaymentTransactionID *uuid.UUID `json:"payment_transaction_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
}

// CategoryProvision represents budgeted amounts per category
type CategoryProvision struct {
	ID                    uuid.UUID              `json:"id"`
	UserID                uuid.UUID              `json:"user_id"`
	CategoryID            uuid.UUID              `json:"category_id"`
	Year                  int                    `json:"year"`
	Month                 int                    `json:"month"`
	EstimatedAmount       float64                `json:"estimated_amount"`
	RepeatForFutureMonths bool                   `json:"repeat_for_future_months"`
	MonthlyOverrides      map[string]interface{} `json:"monthly_overrides,omitempty"`
	ExcludedYearMonthKeys []int                  `json:"excluded_year_month_keys,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
	DeletedAt             *time.Time             `json:"deleted_at,omitempty"`
}

// CategoryQuotaRule represents quota tracking for specific items
type CategoryQuotaRule struct {
	ID                    uuid.UUID              `json:"id"`
	SeriesID              uuid.UUID              `json:"series_id"`
	UserID                uuid.UUID              `json:"user_id"`
	CategoryID            uuid.UUID              `json:"category_id"`
	Name                  string                 `json:"name"`
	IconSymbol            string                 `json:"icon_symbol"`
	Year                  int                    `json:"year"`
	Month                 int                    `json:"month"`
	PlannedQuantity       int                    `json:"planned_quantity"`
	EstimatedUnitPrice    float64                `json:"estimated_unit_price"`
	RepeatForFutureMonths bool                   `json:"repeat_for_future_months"`
	MonthlyOverrides      map[string]interface{} `json:"monthly_overrides,omitempty"`
	ExcludedYearMonthKeys []int                  `json:"excluded_year_month_keys,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
	DeletedAt             *time.Time             `json:"deleted_at,omitempty"`
}

// ChatMessage represents a conversational interaction with the financial assistant
type ChatMessage struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	Role      string     `json:"role"` // "user", "assistant", "system"
	Content   string     `json:"content"`
	Timestamp time.Time  `json:"timestamp"`
	ModelUsed *string    `json:"model_used,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
