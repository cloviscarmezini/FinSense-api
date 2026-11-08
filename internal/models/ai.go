package models

import (
	"time"

	"github.com/google/uuid"
)

type AIModel struct {
	ID             string    `json:"id"`
	DisplayName    string    `json:"display_name"`
	RepoID         string    `json:"repo_id"`
	FileName       string    `json:"file_name"`
	LocalPath      string    `json:"local_path"`
	SizeBytes      int64     `json:"size_bytes"`
	Quantization   string    `json:"quantization"`
	IsGlobalActive bool      `json:"is_global_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AIDownloadTask struct {
	ID                 uuid.UUID  `json:"task_id"`
	UserID             *uuid.UUID `json:"user_id,omitempty"`
	RepoID             string     `json:"repo_id"`
	FileName           string     `json:"file_name"`
	Status             string     `json:"status"` // PENDING, DOWNLOADING, VERIFYING, READY, FAILED, CANCELED
	ProgressPercentage float64    `json:"progress_percentage"`
	DownloadedBytes    int64      `json:"downloaded_bytes"`
	TotalBytes         int64      `json:"total_bytes"`
	SpeedBytesSec      int64      `json:"speed_bytes_sec"`
	ErrorMessage       *string    `json:"error_message"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type InstalledModelInfo struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"display_name"`
	RepoID       string    `json:"repo_id"`
	FileName     string    `json:"file_name"`
	SizeBytes    int64     `json:"size_bytes"`
	Quantization string    `json:"quantization"`
	IsActive     bool      `json:"is_active"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type SuggestedModel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	SizeEstimate string `json:"size_estimate"`
	IsDownloaded bool   `json:"is_downloaded"`
}

type AIModelsResponse struct {
	SelectedModelID *string              `json:"selected_model_id"`
	InstalledModels []InstalledModelInfo `json:"installed_models"`
	SuggestedModels []SuggestedModel     `json:"suggested_models"`
}

type ModelDownloadRequest struct {
	RepoID            string `json:"repo_id"`
	HuggingFaceToken  string `json:"hugging_face_token,omitempty"`
	FileNameFilter    string `json:"filename_filter,omitempty"`
}

type ModelDownloadResponse struct {
	TaskID  uuid.UUID `json:"task_id"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
}

type SelectModelRequest struct {
	ModelID string `json:"model_id"`
}

type SelectModelResponse struct {
	SelectedModelID string `json:"selected_model_id"`
	IsActive        bool   `json:"is_active"`
}

type ChatMessageInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionsRequest struct {
	Messages    []ChatMessageInput `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type ChatMessageChoice struct {
	Index        int              `json:"index"`
	Message      ChatMessageInput `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionsResponse struct {
	ID      string              `json:"id"`
	Model   string              `json:"model"`
	Created int64               `json:"created"`
	Choices []ChatMessageChoice `json:"choices"`
	Usage   ChatUsage           `json:"usage"`
}

type AnalyzeFinancesRequest struct {
	TargetMonth        string     `json:"target_month"` // e.g. "2026-11"
	SpecificDebtID     *uuid.UUID `json:"specific_debt_id,omitempty"`
	SpecificContractID *uuid.UUID `json:"specific_contract_id,omitempty"`
}

type AnalyzeFinancesResponse struct {
	AnalysisDate             time.Time     `json:"analysis_date"`
	NetMonthlyCapacity       float64       `json:"net_monthly_capacity"`
	SafePayoffDate           string        `json:"safe_payoff_date"`
	IsPayoffViableThisMonth  bool          `json:"is_payoff_viable_this_month"`
	RequiredReserveAmount    float64       `json:"required_reserve_amount"`
	ActionPlanMarkdown       string        `json:"action_plan_markdown"`
	ContractsDisambiguation  []interface{} `json:"contracts_disambiguation"`
}

type ParseStatementRequest struct {
	RawText          string     `json:"raw_text"`
	DefaultAccountID *uuid.UUID `json:"default_account_id,omitempty"`
	FilterMode       string     `json:"filter_mode,omitempty"`
}

type ParsedTransaction struct {
	Date                  string  `json:"date"`
	RawDescription        string  `json:"raw_description"`
	CleanTitle            string  `json:"clean_title"`
	Details               string  `json:"details"`
	Amount                float64 `json:"amount"`
	Type                  string  `json:"type"` // "expense" or "income"
	SuggestedCategoryName string  `json:"suggested_category_name"`
	RecurrenceMode        string  `json:"recurrence_mode"`
	InstallmentsCount     int     `json:"installments_count"`
	OccurrenceIndex       int     `json:"occurrence_index"`
}

type ParseStatementResponse struct {
	ParsedTransactions []ParsedTransaction `json:"parsed_transactions"`
}
