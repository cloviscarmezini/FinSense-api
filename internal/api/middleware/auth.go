package middleware

import (
	"context"
	"net/http"
	"strings"

	"finsense-api/internal/config"
	"finsense-api/internal/utils"

	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDContextKey contextKey = "user_id"
	EmailContextKey  contextKey = "email"
)

func Auth(cfg *config.Config) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Cabeçalho de autorização não fornecido.", nil)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthUnauthorized, "Formato de token Bearer inválido.", nil)
				return
			}

			tokenString := parts[1]
			claims, err := utils.ValidateToken(tokenString, cfg.JWTSecret, "access")
			if err != nil {
				utils.RespondWithError(w, http.StatusUnauthorized, utils.ErrAuthTokenExpired, "Token expirado ou inválido. Por favor, renove sua autenticação.", nil)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailContextKey, claims.Email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	val := ctx.Value(UserIDContextKey)
	if id, ok := val.(uuid.UUID); ok {
		return id, true
	}
	return uuid.Nil, false
}
