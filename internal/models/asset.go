package models

import (
	"time"

	"github.com/google/uuid"
)

type AssetFile struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	FileName   string     `json:"file_name"`
	MimeType   string     `json:"mime_type"`
	SizeBytes  int64      `json:"size_bytes"`
	SHA256Hash string     `json:"sha256_hash"`
	StorageKey string     `json:"storage_key"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type PresignedUploadRequest struct {
	FileName  string `json:"file_name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

type PresignedUploadResponse struct {
	AssetID          uuid.UUID `json:"asset_id"`
	StorageKey       string    `json:"storage_key"`
	UploadURL        string    `json:"upload_url"`
	ExpiresInSeconds int       `json:"expires_in_seconds"`
}

type ConfirmUploadRequest struct {
	AssetID    uuid.UUID `json:"asset_id"`
	StorageKey string    `json:"storage_key"`
	SHA256Hash string    `json:"sha256_hash"`
}

type DownloadUrlResponse struct {
	AssetID          uuid.UUID `json:"asset_id"`
	DownloadURL      string    `json:"download_url"`
	ExpiresInSeconds int       `json:"expires_in_seconds"`
}
