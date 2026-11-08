package handlers

import (
	"context"
	"net/http"
	"time"

	"finsense-api/internal/database"
	"finsense-api/internal/utils"
)

type HealthHandler struct {
	db *database.DB
}

func NewHealthHandler(db *database.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbStatus := "healthy"
	if err := h.db.Ping(ctx); err != nil {
		dbStatus = "unreachable"
	}

	status := http.StatusOK
	if dbStatus != "healthy" {
		status = http.StatusServiceUnavailable
	}

	utils.RespondWithJSON(w, status, map[string]interface{}{
		"status":    "ok",
		"service":   "finsense-api",
		"version":   "2.1.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"database":  dbStatus,
	})
}
