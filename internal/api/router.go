package api

import (
	"net/http"

	"finsense-api/internal/api/handlers"
	"finsense-api/internal/api/middleware"
	"finsense-api/internal/config"
	"finsense-api/internal/database"
	"finsense-api/internal/services"

	"github.com/go-chi/chi/v5"
)

type RouterDependencies struct {
	Config         *config.Config
	DB             *database.DB
	StorageService *services.StorageService
	AuthService    *services.AuthService
	SyncService    *services.SyncService
	AIService      *services.AIService
}

func SetupRouter(deps *RouterDependencies) http.Handler {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(middleware.Recover)
	r.Use(middleware.Logger)
	r.Use(middleware.CORS())

	// Handlers
	healthHandler := handlers.NewHealthHandler(deps.DB)
	authHandler := handlers.NewAuthHandler(deps.AuthService)
	syncHandler := handlers.NewSyncHandler(deps.SyncService)
	assetsHandler := handlers.NewAssetsHandler(deps.DB, deps.StorageService)
	backupHandler := handlers.NewBackupHandler(deps.DB)
	aiHandler := handlers.NewAIHandler(deps.AIService)
	txHandler := handlers.NewTransactionHandler(deps.SyncService)
	swaggerHandler := handlers.NewSwaggerHandler("docs/openapi.json", "docs/swagger.html")

	// Root & Health
	r.Get("/", healthHandler.Health)
	r.Get("/health", healthHandler.Health)

	// Swagger & OpenAPI
	if deps.Config.SwaggerEnabled {
		r.Get("/docs", swaggerHandler.ServeUI)
		r.Get("/swagger", swaggerHandler.ServeUI)
		r.Get("/openapi.json", swaggerHandler.ServeOpenAPI)
		r.Get("/api/v1/openapi.json", swaggerHandler.ServeOpenAPI)
	}

	// API v1 Subrouter
	r.Route("/api/v1", func(v1 chi.Router) {
		// Public Auth
		v1.Post("/auth/register", authHandler.Register)
		v1.Post("/auth/login", authHandler.Login)
		v1.Post("/auth/refresh", authHandler.RefreshToken)

		// Protected Routes
		v1.Group(func(protected chi.Router) {
			protected.Use(middleware.Auth(deps.Config))

			// Auth / User
			protected.Post("/auth/logout", authHandler.Logout)
			protected.Get("/user/me", authHandler.Me)

			// Two-Way Sync
			protected.Post("/sync/push-pull", syncHandler.PushPull)
			protected.Get("/sync/delta", syncHandler.Delta)
			protected.Post("/sync/reset", syncHandler.Reset)

			// Transactions & Financial Entities (Direct API Access)
			protected.Get("/transactions", txHandler.List)
			protected.Post("/transactions", txHandler.Create)
			protected.Delete("/transactions/{id}", txHandler.Delete)
			protected.Get("/accounts", txHandler.ListAccounts)
			protected.Get("/categories", txHandler.ListCategories)

			// Assets
			protected.Post("/assets/presigned-upload", assetsHandler.PresignedUpload)
			protected.Post("/assets/confirm-upload", assetsHandler.ConfirmUpload)
			protected.Get("/assets/{id}/download-url", assetsHandler.DownloadURL)

			// Backup
			protected.Get("/backup/export-full", backupHandler.ExportFull)
			protected.Post("/backup/import-selective", backupHandler.ImportSelective)

			// Artificial Intelligence (AI Engine para Apps Móveis / Usuário Final)
			// O modelo utilizado é definido globalmente no microserviço administrativo (finsense-ai-manager)
			protected.Post("/ai/chat/completions", aiHandler.ChatCompletions)
			protected.Post("/ai/analyze-finances", aiHandler.AnalyzeFinances)
			protected.Post("/ai/parse-statement", aiHandler.ParseStatement)
		})
	})

	return r
}
