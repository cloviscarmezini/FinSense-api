package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/models"
	"finsense-api/internal/services"
	"finsense-api/internal/utils"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Corpo da requisição JSON inválido.", nil)
		return
	}

	if req.Email == "" || req.Password == "" || req.FullName == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Os campos email, password e full_name são obrigatórios.", nil)
		return
	}

	res, err := h.authService.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, services.ErrUserAlreadyExists) {
			utils.RespondWithError(w, http.StatusConflict, utils.ErrConflict, "Já existe um usuário cadastrado com este e-mail.", nil)
			return
		}
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao registrar usuário: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusCreated, res)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Corpo da requisição JSON inválido.", nil)
		return
	}

	if req.Email == "" || req.Password == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Os campos email e password são obrigatórios.", nil)
		return
	}

	res, err := h.authService.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthInvalidCredentials, "Email ou senha incorretos.", nil)
			return
		}
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao realizar login: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, res)
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req models.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Corpo da requisição JSON inválido.", nil)
		return
	}

	if req.RefreshToken == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "O campo refresh_token é obrigatório.", nil)
		return
	}

	res, err := h.authService.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthTokenExpired, "Refresh token inválido ou expirado.", nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, res)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Stateless JWT logout confirmation
	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Sessão encerrada com sucesso.",
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	user, err := h.authService.GetUserByID(r.Context(), userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrNotFound, "Usuário não encontrado.", nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, user)
}
