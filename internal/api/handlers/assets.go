package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/database"
	"finsense-api/internal/models"
	"finsense-api/internal/services"
	"finsense-api/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AssetsHandler struct {
	db             *database.DB
	storageService *services.StorageService
}

func NewAssetsHandler(db *database.DB, storageService *services.StorageService) *AssetsHandler {
	return &AssetsHandler{db: db, storageService: storageService}
}

func (h *AssetsHandler) PresignedUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.PresignedUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	assetID := uuid.New()
	ext := filepath.Ext(req.FileName)
	storageKey := fmt.Sprintf("receipts/%s/%s%s", userID.String(), assetID.String(), ext)

	uploadURL, err := h.storageService.GeneratePresignedUploadURL(r.Context(), storageKey, 15*time.Minute)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao gerar URL de upload: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, models.PresignedUploadResponse{
		AssetID:          assetID,
		StorageKey:       storageKey,
		UploadURL:        uploadURL,
		ExpiresInSeconds: 900,
	})
}

func (h *AssetsHandler) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.ConfirmUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	exists, size, err := h.storageService.VerifyObjectExists(r.Context(), req.StorageKey)
	if err != nil || !exists {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Arquivo não encontrado no storage ou upload ainda pendente.", nil)
		return
	}

	now := time.Now().UTC()
	_, err = h.db.Pool.Exec(r.Context(), `
		INSERT INTO asset_files (id, user_id, file_name, mime_type, size_bytes, sha256_hash, storage_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (id) DO UPDATE SET
			storage_key = EXCLUDED.storage_key,
			updated_at = EXCLUDED.updated_at
	`, req.AssetID, userID, filepath.Base(req.StorageKey), "application/octet-stream", size, req.SHA256Hash, req.StorageKey, now)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao registrar asset: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"asset_id":   req.AssetID,
		"status":     "confirmed",
		"size_bytes": size,
	})
}

func (h *AssetsHandler) DownloadURL(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	idStr := chi.URLParam(r, "id")
	assetID, err := uuid.Parse(idStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "UUID de asset inválido.", nil)
		return
	}

	var storageKey string
	err = h.db.Pool.QueryRow(r.Context(), "SELECT storage_key FROM asset_files WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL", assetID, userID).Scan(&storageKey)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrNotFound, "Asset não encontrado.", nil)
		return
	}

	downloadURL, err := h.storageService.GeneratePresignedDownloadURL(r.Context(), storageKey, 30*time.Minute)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao gerar URL de download: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, models.DownloadUrlResponse{
		AssetID:          assetID,
		DownloadURL:      downloadURL,
		ExpiresInSeconds: 1800,
	})
}
