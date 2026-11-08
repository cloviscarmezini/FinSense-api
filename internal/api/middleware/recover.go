package middleware

import (
	"log"
	"net/http"

	"finsense-api/internal/utils"
)

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC RECOVERED] %v", rec)
				utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Ocorreu um erro interno inesperado no servidor.", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
