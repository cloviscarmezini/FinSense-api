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
	CurrencyCode    string     `json:"currency_code"`
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
	TransactionOrigin                     *string    `json:"transaction_origin,omitempty"`
	LinkedEntityID                        *uuid.UUID `json:"linked_entity_id,omitempty"`
	BaseInstallmentRegisteredAmount       *float64   `json:"base_installment_registered_amount,omitempty"`
	ProjectedInstallmentCorrectionAmount  *float64   `json:"projected_installment_correction_amount,omitempty"`
	ConfirmedRealInterestCorrectionAmount *float64   `json:"confirmed_real_interest_correction_amount,omitempty"`
	CreditCardID                          *uuid.UUID `json:"credit_card_id,omitempty"`
	CreditCardInvoiceYearMonthKey         *int       `json:"credit_card_invoice_year_month_key,omitempty"`
	CreditCardInvoiceID                   *uuid.UUID `json:"credit_card_invoice_id,omitempty"`
	NeedsReview                           bool       `json:"needs_review"`
	SourceApp                             *string    `json:"source_app,omitempty"`
	IsReimbursable                        bool       `json:"is_reimbursable"`
	ReimbursableDebtor                    *string    `json:"reimbursable_debtor,omitempty"`
	ReimbursableAmount                    *float64   `json:"reimbursable_amount,omitempty"`
	ReimbursementExpectedDate             *time.Time `json:"reimbursement_expected_date,omitempty"`
	ReimbursementReceivedAmount           *float64   `json:"reimbursement_received_amount,omitempty"`
	IsReimbursementSettled                bool       `json:"is_reimbursement_settled"`
	LinkedReimbursementTransactionID      *uuid.UUID `json:"linked_reimbursement_transaction_id,omitempty"`
	CurrencyCode                          string     `json:"currency_code"`
	ExchangeRateToBRL                     *float64   `json:"exchange_rate_to_brl,omitempty"`
	CreatedAt                             time.Time  `json:"created_at"`
	UpdatedAt                             time.Time  `json:"updated_at"`
	DeletedAt                             *time.Time `json:"deleted_at,omitempty"`
}

// CreditCard represents a credit card entity synced with client devices
type CreditCard struct {
	ID                      uuid.UUID  `json:"id"`
	UserID                  uuid.UUID  `json:"user_id"`
	Name                    string     `json:"name"`
	TotalLimit              float64    `json:"total_limit"`
	ClosingDay              int        `json:"closing_day"`
	BestPurchaseDay         int        `json:"best_purchase_day"`
	DueDay                  int        `json:"due_day"`
	DefaultPaymentAccountID *uuid.UUID `json:"default_payment_account_id,omitempty"`
	Brand                   string     `json:"brand"`
	BankPresetID            *string    `json:"bank_preset_id,omitempty"`
	CustomImagePath         *string    `json:"custom_image_path,omitempty"`
	IconSymbol              string     `json:"icon_symbol"`
	ColorHex                string     `json:"color_hex"`
	LastFourDigits          string     `json:"last_four_digits"`
	DisplayOrder            int        `json:"display_order"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	DeletedAt               *time.Time `json:"deleted_at,omitempty"`
}

// CreditCardInvoice represents a monthly billing cycle for a credit card
type CreditCardInvoice struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               uuid.UUID  `json:"user_id"`
	CreditCardID         uuid.UUID  `json:"credit_card_id"`
	Year                 int        `json:"year"`
	Month                int        `json:"month"`
	ClosingDate          time.Time  `json:"closing_date"`
	DueDate              time.Time  `json:"due_date"`
	IsPaid               bool       `json:"is_paid"`
	PaidAmount           *float64   `json:"paid_amount,omitempty"`
	PaidDate             *time.Time `json:"paid_date,omitempty"`
	PaidFromAccountID    *uuid.UUID `json:"paid_from_account_id,omitempty"`
	PaymentTransactionID *uuid.UUID `json:"payment_transaction_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
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
	CurrencyCode         string      `json:"currency_code"`
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
	SettledDate                       *time.Time  `json:"settled_date,omitempty"`
	SettledAccountID                  *uuid.UUID  `json:"settled_account_id,omitempty"`
	BankDueDay                        *int        `json:"bank_due_day,omitempty"`
	ReferenceDate                     *time.Time  `json:"reference_date,omitempty"`
	MaturityDate                      *time.Time  `json:"maturity_date,omitempty"`
	EstimatedMonthlyTR                *float64    `json:"estimated_monthly_tr,omitempty"`
	LastFGTSUsageDate                 *time.Time  `json:"last_fgts_usage_date,omitempty"`
	ScheduledInstallments             interface{} `json:"scheduled_installments,omitempty"`
	AverageMonthlyInstallmentGrowth   *float64    `json:"average_monthly_installment_growth,omitempty"`
	Notes                             string      `json:"notes"`
	CreatedAt                         time.Time   `json:"created_at"`
	UpdatedAt                         time.Time   `json:"updated_at"`
	DeletedAt                         *time.Time  `json:"deleted_at,omitempty"`
}

// AmortizationPlan represents a dynamic amortization planning configuration (Layer 2) for a credit contract
type AmortizationPlan struct {
	ID                         uuid.UUID   `json:"id"`
	UserID                     uuid.UUID   `json:"user_id"`
	ContractID                 uuid.UUID   `json:"contract_id"`
	GenerationMode             string      `json:"generation_mode"`
	BillingDay                 int         `json:"billing_day"`
	IncomeCommitmentPercentage float64     `json:"income_commitment_percentage"`
	DefaultRecalculationMode   string      `json:"default_recalculation_mode"`
	EstimatedMonthlyTR         float64     `json:"estimated_monthly_tr"`
	Items                      interface{} `json:"items,omitempty"`
	CreatedAt                  time.Time   `json:"created_at"`
	UpdatedAt                  time.Time   `json:"updated_at"`
	DeletedAt                  *time.Time  `json:"deleted_at,omitempty"`
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
	CategoryID            *uuid.UUID             `json:"category_id,omitempty"`
	Name                  string                 `json:"name"`
	IconSymbol            string                 `json:"icon_symbol"`
	Year                  int                    `json:"year"`
	Month                 int                    `json:"month"`
	PlannedQuantity       int                    `json:"planned_quantity"`
	EstimatedUnitPrice    float64                `json:"estimated_unit_price"`
	RepeatForFutureMonths bool                   `json:"repeat_for_future_months"`
	MonthlyOverrides      map[string]interface{} `json:"monthly_overrides,omitempty"`
	ExcludedYearMonthKeys []int                  `json:"excluded_year_month_keys,omitempty"`
	QuotaType             string                 `json:"quota_type"`
	LinkedInvestmentID    *uuid.UUID             `json:"linked_investment_id,omitempty"`
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

// SinkingFund represents an annual or seasonal proportional provision
type SinkingFund struct {
	ID                 uuid.UUID  `json:"id"`
	UserID             uuid.UUID  `json:"user_id"`
	Name               string     `json:"name"`
	CategoryID         *uuid.UUID `json:"category_id,omitempty"`
	IconSymbol         string     `json:"icon_symbol"`
	ColorHex           string     `json:"color_hex"`
	TargetAmount       float64    `json:"target_amount"`
	RecurrenceMonths   int        `json:"recurrence_months"`
	DueMonth           int        `json:"due_month"`
	DueYear            int        `json:"due_year"`
	AccumulatedReserve float64    `json:"accumulated_reserve"`
	IsEnabled          bool       `json:"is_enabled"`
	Notes              string     `json:"notes"`
	CurrencyCode       string     `json:"currency_code"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

// WhatIfScenario represents a saved cash stress test / decision scenario
type WhatIfScenario struct {
	ID                             uuid.UUID  `json:"id"`
	UserID                         uuid.UUID  `json:"user_id"`
	Name                           string     `json:"name"`
	VariableIncomeReductionPercent float64    `json:"variable_income_reduction_percent"`
	UnexpectedExpenseAmount        float64    `json:"unexpected_expense_amount"`
	UnexpectedExpenseMonthOffset   int        `json:"unexpected_expense_month_offset"`
	BenchmarkAnnualRatePercent     float64    `json:"benchmark_annual_rate_percent"`
	NetYieldTaxRatePercent         float64    `json:"net_yield_tax_rate_percent"`
	Notes                          string     `json:"notes"`
	CreatedAt                      time.Time  `json:"created_at"`
	UpdatedAt                      time.Time  `json:"updated_at"`
	DeletedAt                      *time.Time `json:"deleted_at,omitempty"`
}

// FinancialInsight represents a proactive ML behavioral insight
type FinancialInsight struct {
	ID                   uuid.UUID  `json:"id"`
	UserID               uuid.UUID  `json:"user_id"`
	Fingerprint          string     `json:"fingerprint"`
	Kind                 string     `json:"kind"`
	Severity             string     `json:"severity"`
	Title                string     `json:"title"`
	Message              string     `json:"message"`
	Recommendation       string     `json:"recommendation"`
	ImpactAmount         float64    `json:"impact_amount"`
	ConfidenceScore      float64    `json:"confidence_score"`
	YearMonthKey         int        `json:"year_month_key"`
	RelatedCategoryID    *uuid.UUID `json:"related_category_id,omitempty"`
	RelatedTransactionID *uuid.UUID `json:"related_transaction_id,omitempty"`
	Feedback             string     `json:"feedback"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
}

// MonthlyReview represents a guided monthly closing record
type MonthlyReview struct {
	ID                         uuid.UUID   `json:"id"`
	UserID                     uuid.UUID   `json:"user_id"`
	Year                       int         `json:"year"`
	Month                      int         `json:"month"`
	PlannedIncome              float64     `json:"planned_income"`
	RealizedIncome             float64     `json:"realized_income"`
	PlannedExpense             float64     `json:"planned_expense"`
	RealizedExpense            float64     `json:"realized_expense"`
	PlannedEndBalance          float64     `json:"planned_end_balance"`
	RealizedEndBalance         float64     `json:"realized_end_balance"`
	PreviousMonthEndBalance    float64     `json:"previous_month_end_balance"`
	PendingAmortizationsCount  int         `json:"pending_amortizations_count"`
	PendingAmortizationsAmount float64     `json:"pending_amortizations_amount"`
	DeviationHighlights        interface{} `json:"deviation_highlights,omitempty"`
	IsCompleted                bool        `json:"is_completed"`
	CompletedAt                *time.Time  `json:"completed_at,omitempty"`
	Notes                      string      `json:"notes"`
	CreatedAt                  time.Time   `json:"created_at"`
	UpdatedAt                  time.Time   `json:"updated_at"`
	DeletedAt                  *time.Time  `json:"deleted_at,omitempty"`
}

// MLBehavioralProfile stores learned continuous ML weights and baselines
type MLBehavioralProfile struct {
	ID                               uuid.UUID   `json:"id"`
	UserID                           uuid.UUID   `json:"user_id"`
	LastTrainedAt                    *time.Time  `json:"last_trained_at,omitempty"`
	TrainingEpochCount               int         `json:"training_epoch_count"`
	AnalyzedTransactionsCount        int         `json:"analyzed_transactions_count"`
	InflationSensitivity             float64     `json:"inflation_sensitivity"`
	AnomalyZThreshold                float64     `json:"anomaly_z_threshold"`
	SubscriptionDriftThresholdPercent float64    `json:"subscription_drift_threshold_percent"`
	CategoryWeightAdjustments        interface{} `json:"category_weight_adjustments,omitempty"`
	DayOfMonthSpendCurve             interface{} `json:"day_of_month_spend_curve,omitempty"`
	CategoryStats                    interface{} `json:"category_stats,omitempty"`
	UsefulFeedbackCount              int         `json:"useful_feedback_count"`
	IgnoredFeedbackCount             int         `json:"ignored_feedback_count"`
	DismissedFeedbackCount           int         `json:"dismissed_feedback_count"`
	CreatedAt                        time.Time   `json:"created_at"`
	UpdatedAt                        time.Time   `json:"updated_at"`
	DeletedAt                        *time.Time  `json:"deleted_at,omitempty"`
}

// ReconciliationRule represents a deterministic rule to classify and enrich transactions
type ReconciliationRule struct {
	ID                       uuid.UUID  `json:"id"`
	UserID                   uuid.UUID  `json:"user_id"`
	Name                     string     `json:"name"`
	Priority                 int        `json:"priority"`
	IsEnabled                bool       `json:"is_enabled"`
	MatchField               string     `json:"match_field"`
	MatchOperator            string     `json:"match_operator"`
	MatchPattern             string     `json:"match_pattern"`
	MinAmount                *float64   `json:"min_amount,omitempty"`
	MaxAmount                *float64   `json:"max_amount,omitempty"`
	SourceAccountID          *uuid.UUID `json:"source_account_id,omitempty"`
	SourceCreditCardID       *uuid.UUID `json:"source_credit_card_id,omitempty"`
	TargetCategoryID         *uuid.UUID `json:"target_category_id,omitempty"`
	CleanName                *string    `json:"clean_name,omitempty"`
	MarkRecurring            *bool      `json:"mark_recurring,omitempty"`
	MarkReimbursable         *bool      `json:"mark_reimbursable,omitempty"`
	ReimbursableDebtor       *string    `json:"reimbursable_debtor,omitempty"`
	MarkIgnoreInConsolidated *bool      `json:"mark_ignore_in_consolidated,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	DeletedAt                *time.Time `json:"deleted_at,omitempty"`
}

// AssetItem represents a non-bank physical or financial asset (real estate, vehicle, FGTS, etc.)
type AssetItem struct {
	ID                           uuid.UUID   `json:"id"`
	UserID                       uuid.UUID   `json:"user_id"`
	Name                         string      `json:"name"`
	AssetClass                   string      `json:"asset_class"`
	PurchaseValue                float64     `json:"purchase_value"`
	CurrentMarketValue           float64     `json:"current_market_value"`
	AcquisitionDate              *time.Time  `json:"acquisition_date,omitempty"`
	LinkedFinancingContractID    *uuid.UUID  `json:"linked_financing_contract_id,omitempty"`
	AnnualDepreciationRatePercent *float64   `json:"annual_depreciation_rate_percent,omitempty"`
	MonthlyContributionEstimate  *float64    `json:"monthly_contribution_estimate,omitempty"`
	CurrencyCode                 string      `json:"currency_code"`
	ValuationHistory             interface{} `json:"valuation_history,omitempty"`
	Notes                        string      `json:"notes"`
	CreatedAt                    time.Time   `json:"created_at"`
	UpdatedAt                    time.Time   `json:"updated_at"`
	DeletedAt                    *time.Time  `json:"deleted_at,omitempty"`
}

// IncomeSource represents a structured recurring/seasonal/passive income stream
type IncomeSource struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	Name            string     `json:"name"`
	Kind            string     `json:"kind"`
	GrossAmount     float64    `json:"gross_amount"`
	NetAmount       float64    `json:"net_amount"`
	PaymentDay      *int       `json:"payment_day,omitempty"`
	ExpectedMonth   *int       `json:"expected_month,omitempty"`
	Frequency       string     `json:"frequency"`
	PayerName       string     `json:"payer_name"`
	LinkedAccountID *uuid.UUID `json:"linked_account_id,omitempty"`
	CurrencyCode    string     `json:"currency_code"`
	IsActive        bool       `json:"is_active"`
	Notes           string     `json:"notes"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

// NetWorthSnapshot records historical monthly Net Worth telemetry
type NetWorthSnapshot struct {
	ID                               uuid.UUID  `json:"id"`
	UserID                           uuid.UUID  `json:"user_id"`
	YearMonthKey                     int        `json:"year_month_key"`
	SnapshotDate                     time.Time  `json:"snapshot_date"`
	LiquidAssetsBRL                  float64    `json:"liquid_assets_brl"`
	InvestmentsAssetsBRL             float64    `json:"investments_assets_brl"`
	RealEstateAssetsBRL              float64    `json:"real_estate_assets_brl"`
	VehicleAssetsBRL                 float64    `json:"vehicle_assets_brl"`
	FGTSAndOtherAssetsBRL            float64    `json:"fgts_and_other_assets_brl"`
	TotalAssetsBRL                   float64    `json:"total_assets_brl"`
	LoansAndFinancingsLiabilitiesBRL float64    `json:"loans_and_financings_liabilities_brl"`
	CreditCardsLiabilitiesBRL        float64    `json:"credit_cards_liabilities_brl"`
	DebtsLiabilitiesBRL              float64    `json:"debts_liabilities_brl"`
	TotalLiabilitiesBRL              float64    `json:"total_liabilities_brl"`
	NetWorthBRL                      float64    `json:"net_worth_brl"`
	CreatedAt                        time.Time  `json:"created_at"`
	UpdatedAt                        time.Time  `json:"updated_at"`
	DeletedAt                        *time.Time `json:"deleted_at,omitempty"`
}

// FIREScenario stores a saved financial independence simulation scenario
type FIREScenario struct {
	ID                                uuid.UUID  `json:"id"`
	UserID                            uuid.UUID  `json:"user_id"`
	Name                              string     `json:"name"`
	MonthlyCostOfLivingBRL            float64    `json:"monthly_cost_of_living_brl"`
	CurrentInvestedPatrimonyBRL       float64    `json:"current_invested_patrimony_brl"`
	IncludeRealEstateAndFGTS          bool       `json:"include_real_estate_and_fgts"`
	MonthlyContributionBRL            float64    `json:"monthly_contribution_brl"`
	SafeWithdrawalRatePercent         float64    `json:"safe_withdrawal_rate_percent"`
	ExpectedAnnualNominalReturnPercent float64   `json:"expected_annual_nominal_return_percent"`
	ExpectedAnnualInflationPercent    float64    `json:"expected_annual_inflation_percent"`
	Notes                             string     `json:"notes"`
	CreatedAt                         time.Time  `json:"created_at"`
	UpdatedAt                         time.Time  `json:"updated_at"`
	DeletedAt                         *time.Time `json:"deleted_at,omitempty"`
}
