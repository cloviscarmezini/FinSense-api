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

type TransactionHandler struct {
	syncService *services.SyncService
}

func NewTransactionHandler(syncService *services.SyncService) *TransactionHandler {
	return &TransactionHandler{syncService: syncService}
}

func (h *TransactionHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	list, err := h.syncService.ListTransactions(r.Context(), userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao listar transações: "+err.Error(), nil)
		return
	}

	if list == nil {
		list = []models.FinancialTransaction{}
	}

	utils.RespondWithJSON(w, http.StatusOK, list)
}

func (h *TransactionHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var tx models.FinancialTransaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrSyncInvalidPayload, "Payload de transação inválido.", map[string]any{"raw_error": err.Error()})
		return
	}

	if tx.Name == "" || tx.AccountID == uuid.Nil || tx.CategoryID == uuid.Nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrSyncInvalidPayload, "name, account_id e category_id são obrigatórios.", nil)
		return
	}

	created, err := h.syncService.CreateTransaction(r.Context(), userID, tx)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao criar transação: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusCreated, created)
}

func (h *TransactionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	idStr := chi.URLParam(r, "id")
	txID, err := uuid.Parse(idStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrSyncInvalidPayload, "ID da transação inválido.", nil)
		return
	}

	if err := h.syncService.DeleteTransaction(r.Context(), userID, txID); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao excluir transação: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Transação excluída com sucesso.",
	})
}

func (h *TransactionHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	list, err := h.syncService.ListAccounts(r.Context(), userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao listar contas: "+err.Error(), nil)
		return
	}
	if list == nil {
		list = []models.BankAccount{}
	}

	utils.RespondWithJSON(w, http.StatusOK, list)
}

func (h *TransactionHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	list, err := h.syncService.ListCategories(r.Context(), userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao listar categorias: "+err.Error(), nil)
		return
	}
	if list == nil {
		list = []models.Category{}
	}

	utils.RespondWithJSON(w, http.StatusOK, list)
}
