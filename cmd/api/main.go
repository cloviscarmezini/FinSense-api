package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"finsense-api/internal/api"
	"finsense-api/internal/config"
	"finsense-api/internal/database"
	"finsense-api/internal/services"
)

func main() {
	log.Println("==================================================")
	log.Println("  FinSense Backend & Two-Way Sync Service v2.1.0  ")
	log.Println("==================================================")

	// 1. Carregar Configurações
	cfg := config.Load()
	log.Printf("[CONFIG] Ambiente: %s | Porta: %s", cfg.Environment, cfg.Port)

	// 2. Conectar ao Banco de Dados PostgreSQL
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Erro ao conectar ao banco de dados: %v", err)
	}
	defer db.Close()

	// 3. Inicializar Serviços de Domínio
	storageService := services.NewStorageService(cfg)
	authService := services.NewAuthService(db, cfg)
	syncService := services.NewSyncService(db)
	aiService := services.NewAIService(db, cfg)

	// 4. Configurar Roteador HTTP
	router := api.SetupRouter(&api.RouterDependencies{
		Config:         cfg,
		DB:             db,
		StorageService: storageService,
		AuthService:    authService,
		SyncService:    syncService,
		AIService:      aiService,
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 5. Iniciar Servidor em Goroutine
	go func() {
		log.Printf("[SERVER] Servidor FinSense iniciado em http://0.0.0.0:%s", cfg.Port)
		if cfg.SwaggerEnabled {
			log.Printf("[SWAGGER] Swagger UI disponível em http://localhost:%s/docs", cfg.Port)
			log.Printf("[OPENAPI] Especificação OpenAPI em http://localhost:%s/api/v1/openapi.json", cfg.Port)
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Erro na execução do servidor HTTP: %v", err)
		}
	}()

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[SERVER] Encerrando servidor graciosamente...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Forçado o encerramento do servidor: %v", err)
	}

	log.Println("[SERVER] Servidor finalizado com sucesso.")
}
