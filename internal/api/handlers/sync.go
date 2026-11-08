package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/models"
	"finsense-api/internal/services"
	"finsense-api/internal/utils"
)

type SyncHandler struct {
	syncService *services.SyncService
}

func NewSyncHandler(syncService *services.SyncService) *SyncHandler {
	return &SyncHandler{syncService: syncService}
}

func (h *SyncHandler) PushPull(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.PushPullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrSyncInvalidPayload, "Payload de sincronização inválido.", map[string]any{"raw_error": err.Error()})
		return
	}

	if req.ClientID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrSyncInvalidPayload, "client_id é obrigatório para sincronização.", nil)
		return
	}

	res, err := h.syncService.PushPull(r.Context(), userID, req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao processar sincronização: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, res)
}

func (h *SyncHandler) Delta(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	sinceRevStr := r.URL.Query().Get("since_revision")
	var sinceRev int64
	if sinceRevStr != "" {
		if val, err := strconv.ParseInt(sinceRevStr, 10, 64); err == nil {
			sinceRev = val
		}
	}

	clientID := r.URL.Query().Get("client_id")

	changes, latestRev, err := h.syncService.GetDelta(r.Context(), userID, clientID, sinceRev)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao consultar delta: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"latest_revision": latestRev,
		"server_changes":  changes,
	})
}

func (h *SyncHandler) Reset(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	if err := h.syncService.ResetUserData(r.Context(), userID); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao resetar dados: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Base de dados do usuário resetada com sucesso.",
	})
}
