package utils

import (
	"encoding/json"
	"net/http"
	"time"
)

// RFC 7807 Standard Error Structure
type APIError struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	StatusCode int            `json:"status_code"`
	Timestamp  string         `json:"timestamp"`
	Details    map[string]any `json:"details,omitempty"`
}

// Standard Error Codes
const (
	ErrAuthInvalidCredentials = "AUTH_INVALID_CREDENTIALS"
	ErrAuthTokenExpired       = "AUTH_TOKEN_EXPIRED"
	ErrAuthUnauthorized       = "AUTH_UNAUTHORIZED"
	ErrSyncInvalidPayload     = "SYNC_INVALID_PAYLOAD"
	ErrAIModelNotFound        = "AI_MODEL_NOT_FOUND"
	ErrAIModelBusy            = "AI_MODEL_BUSY"
	ErrAIDownloadInProgress   = "AI_DOWNLOAD_IN_PROGRESS"
	ErrAIInsufficientStorage  = "AI_INSUFFICIENT_STORAGE"
	ErrInternalServer         = "INTERNAL_SERVER_ERROR"
	ErrNotFound               = "RESOURCE_NOT_FOUND"
	ErrBadRequest             = "BAD_REQUEST"
	ErrConflict               = "RESOURCE_CONFLICT"
)

func RespondWithError(w http.ResponseWriter, statusCode int, code string, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := APIError{
		Error: ErrorDetail{
			Code:       code,
			Message:    message,
			StatusCode: statusCode,
			Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
			Details:    details,
		},
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func RespondWithJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
