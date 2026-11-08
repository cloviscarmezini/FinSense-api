package handlers

import (
	"encoding/json"
	"net/http"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/models"
	"finsense-api/internal/services"
	"finsense-api/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AIHandler struct {
	aiService *services.AIService
}

func NewAIHandler(aiService *services.AIService) *AIHandler {
	return &AIHandler{aiService: aiService}
}

func (h *AIHandler) GetModels(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	res, err := h.aiService.GetModels(r.Context(), userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao listar modelos: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, res)
}

func (h *AIHandler) DownloadModel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.ModelDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	if req.RepoID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "repo_id é obrigatório.", nil)
		return
	}

	res, err := h.aiService.StartModelDownload(r.Context(), userID, req)
	if err != nil {
		utils.RespondWithError(w, http.StatusConflict, utils.ErrAIDownloadInProgress, err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusAccepted, res)
}

func (h *AIHandler) GetDownloadStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	taskIDStr := chi.URLParam(r, "task_id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "task_id inválido.", nil)
		return
	}

	task, err := h.aiService.GetDownloadTask(r.Context(), taskID)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrNotFound, "Tarefa de download não encontrada.", nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, task)
}

func (h *AIHandler) CancelDownload(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	taskIDStr := chi.URLParam(r, "task_id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "task_id inválido.", nil)
		return
	}

	if err := h.aiService.CancelDownload(r.Context(), taskID); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao cancelar download: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"task_id": taskID,
		"status":  "CANCELED",
		"message": "Download cancelado com sucesso.",
	})
}

func (h *AIHandler) SelectModel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.SelectModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	if err := h.aiService.SelectModel(r.Context(), userID, req.ModelID); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrAIModelNotFound, err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, models.SelectModelResponse{
		SelectedModelID: req.ModelID,
		IsActive:        true,
	})
}

func (h *AIHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	modelID := chi.URLParam(r, "model_id")
	if modelID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "model_id é obrigatório.", nil)
		return
	}

	freedBytes, err := h.aiService.DeleteModel(r.Context(), modelID)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrAIModelNotFound, err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"deleted_model_id": modelID,
		"freed_bytes":      freedBytes,
	})
}

func (h *AIHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.ChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	resp, err := h.aiService.ChatCompletions(r.Context(), req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha na inferência de chat: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, resp)
}

func (h *AIHandler) AnalyzeFinances(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.AnalyzeFinancesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	resp, err := h.aiService.AnalyzeFinances(r.Context(), userID, req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha na análise financeira: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, resp)
}

func (h *AIHandler) ParseStatement(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.ParseStatementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	resp, err := h.aiService.ParseStatement(r.Context(), req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao processar extrato: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, resp)
}
