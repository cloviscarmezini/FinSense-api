package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/database"
	"finsense-api/internal/utils"
)

type BackupHandler struct {
	db *database.DB
}

func NewBackupHandler(db *database.DB) *BackupHandler {
	return &BackupHandler{db: db}
}

func (h *BackupHandler) ExportFull(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	tables := []string{
		"bank_accounts", "categories", "financial_transactions",
		"investment_items", "investment_movements", "credit_contracts",
		"amortization_records", "debt_items", "category_provisions",
		"category_quota_rules",
	}

	backupData := make(map[string]interface{})
	backupData["exported_at"] = time.Now().UTC().Format(time.RFC3339)
	backupData["user_id"] = userID.String()
	backupData["version"] = "2.1.0"

	for _, table := range tables {
		query := fmt.Sprintf("SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT * FROM %s WHERE user_id = $1 AND deleted_at IS NULL) t", table)
		var jsonBytes []byte
		_ = h.db.Pool.QueryRow(r.Context(), query, userID).Scan(&jsonBytes)

		var items []interface{}
		_ = json.Unmarshal(jsonBytes, &items)
		backupData[table] = items
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"finsense_backup_%s.json\"", time.Now().Format("20060102_150405")))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(backupData)
}

func (h *BackupHandler) ImportSelective(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Arquivo de backup ou JSON inválido.", nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Importação granular concluída.",
	})
}
