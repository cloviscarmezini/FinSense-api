package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"finsense-api/internal/api/middleware"
	"finsense-api/internal/models"
	"finsense-api/internal/services"
	"finsense-api/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func populateDeviceHeaders(r *http.Request, deviceID **string, platform **string, appVersion **string, deviceName **string) {
	if *deviceID == nil || **deviceID == "" {
		if val := r.Header.Get("X-Device-Id"); val != "" {
			*deviceID = &val
		} else if val := r.Header.Get("X-Client-Id"); val != "" {
			*deviceID = &val
		}
	}
	if *platform == nil || **platform == "" {
		if val := r.Header.Get("X-Platform"); val != "" {
			*platform = &val
		} else {
			ua := r.UserAgent()
			p := "Unknown"
			if strings.Contains(ua, "iPhone") || strings.Contains(ua, "iOS") || strings.Contains(ua, "iPad") {
				p = "iOS"
			} else if strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Mac OS") {
				p = "macOS"
			}
			*platform = &p
		}
	}
	if *appVersion == nil || **appVersion == "" {
		if val := r.Header.Get("X-App-Version"); val != "" {
			*appVersion = &val
		}
	}
	if *deviceName == nil || **deviceName == "" {
		if val := r.Header.Get("X-Device-Name"); val != "" {
			*deviceName = &val
		}
	}
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

	populateDeviceHeaders(r, &req.DeviceID, &req.Platform, &req.AppVersion, &req.DeviceName)

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

	populateDeviceHeaders(r, &req.DeviceID, &req.Platform, &req.AppVersion, &req.DeviceName)

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

	populateDeviceHeaders(r, &req.DeviceID, &req.Platform, &req.AppVersion, &req.DeviceName)

	res, err := h.authService.RefreshToken(r.Context(), req, r.RemoteAddr)
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

func (h *AuthHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	sessions, err := h.authService.ListUserSessions(r.Context(), userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao consultar sessões ativas: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
	})
}

func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Usuário não autenticado.", nil)
		return
	}

	sessionIDStr := chi.URLParam(r, "id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "ID de sessão inválido.", nil)
		return
	}

	if err := h.authService.RevokeSession(r.Context(), userID, sessionID); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao revogar sessão: "+err.Error(), nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Sessão revogada com sucesso.",
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
