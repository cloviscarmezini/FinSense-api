package handlers

import (
	"encoding/json"
	"net/http"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/models"
	"finsense-api/internal/services"
	"finsense-api/internal/utils"
)

type AIHandler struct {
	aiService *services.AIService
}

func NewAIHandler(aiService *services.AIService) *AIHandler {
	return &AIHandler{aiService: aiService}
}

// ChatCompletions responde conversas do usuário autenticado usando o modelo global ativo
// e injetando estritamente os dados financeiros isolados do próprio usuário.
func (h *AIHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.ChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload JSON de chat inválido.", nil)
		return
	}

	resp, err := h.aiService.ChatCompletions(r.Context(), userID, req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha na inferência de chat: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, resp)
}

// AnalyzeFinances gera um diagnóstico financeiro consolidado exclusivamente com dados do usuário logado.
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

// ParseStatement extrai transações de extrato de forma isolada para a conta do usuário logado.
func (h *AIHandler) ParseStatement(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var req models.ParseStatementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	resp, err := h.aiService.ParseStatement(r.Context(), userID, req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao processar extrato: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, resp)
}
