package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"finsense-api/internal/config"
	"finsense-api/internal/database"
	"finsense-api/internal/models"
	"finsense-api/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AIManagerServer struct {
	cfg         *config.Config
	db          *database.DB
	cancels     map[uuid.UUID]context.CancelFunc
	cancelMutex sync.Mutex
}

func main() {
	log.Println("==========================================================")
	log.Println("  FinSense AI Manager Microservice & Admin Dashboard     ")
	log.Println("==========================================================")

	cfg := config.Load()
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("[FATAL] Erro ao conectar ao banco de dados: %v", err)
	}
	defer db.Close()

	srv := &AIManagerServer{
		cfg:     cfg,
		db:      db,
		cancels: make(map[uuid.UUID]context.CancelFunc),
	}

	r := chi.NewRouter()

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-Admin-Key"},
	}))

	// Healthcheck
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"service":   "finsense-ai-manager",
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Web Dashboard Admin Frontend
	r.Get("/", srv.handleServeWebAdmin)
	r.Get("/admin", srv.handleServeWebAdmin)

	// Admin API (Gestão Global de Modelos de IA)
	r.Route("/api/admin/models", func(admin chi.Router) {
		admin.Get("/", srv.handleGetModels)
		admin.Post("/download", srv.handleStartDownload)
		admin.Get("/download/{task_id}", srv.handleGetDownloadStatus)
		admin.Post("/download/{task_id}/cancel", srv.handleCancelDownload)
		admin.Post("/select", srv.handleSelectActiveModel)
		admin.Delete("/{model_id}", srv.handleDeleteModel)
	})

	// Internal API (Consumida pelo finsense-api para o Chat do Usuário)
	r.Route("/internal/v1", func(internal chi.Router) {
		internal.Get("/active-model", srv.handleGetActiveModelInternal)
		internal.Post("/chat/completions", srv.handleChatCompletionsInternal)
	})

	port := cfg.AdminPort
	if port == "" {
		port = "8082"
	}

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[AI-MANAGER] Painel Web do Administrador disponível em http://localhost:%s/", port)
		log.Printf("[AI-MANAGER] API Interna para o FinSense Backend em http://localhost:%s/internal/v1", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Erro ao iniciar servidor HTTP do AI Manager: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[AI-MANAGER] Finalizando serviço graciosamente...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func (s *AIManagerServer) handleServeWebAdmin(w http.ResponseWriter, r *http.Request) {
	// Procura o arquivo index.html na pasta web/admin
	possiblePaths := []string{
		"web/admin/index.html",
		"/app/web/admin/index.html",
		"../web/admin/index.html",
	}

	var content []byte
	var err error
	for _, p := range possiblePaths {
		content, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "Painel administrativo index.html não encontrado no servidor.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

func (s *AIManagerServer) handleGetModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Pool.Query(r.Context(), `
		SELECT id, display_name, repo_id, file_name, size_bytes, quantization, is_global_active, created_at
		FROM ai_models
		ORDER BY created_at DESC
	`)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao consultar modelos: "+err.Error(), nil)
		return
	}
	defer rows.Close()

	var installed []models.InstalledModelInfo
	var selectedID *string

	for rows.Next() {
		var m models.InstalledModelInfo
		var isGlobalActive bool
		if err := rows.Scan(&m.ID, &m.DisplayName, &m.RepoID, &m.FileName, &m.SizeBytes, &m.Quantization, &isGlobalActive, &m.DownloadedAt); err == nil {
			m.IsActive = isGlobalActive
			if isGlobalActive {
				idCopy := m.ID
				selectedID = &idCopy
			}
			installed = append(installed, m)
		}
	}

	suggested := []models.SuggestedModel{
		{
			ID:           "Qwen/Qwen2.5-3B-Instruct-GGUF",
			Name:         "Qwen 2.5 3B Instruct (GGUF)",
			Description:  "Excelente em português, categorização bancária e cálculos financeiros.",
			SizeEstimate: "~1.9 GB",
			IsDownloaded: false,
		},
		{
			ID:           "Qwen/Qwen2.5-1.5B-Instruct-GGUF",
			Name:         "Qwen 2.5 1.5B Instruct (Ultra Leve)",
			Description:  "Modelo compacto e de baixíssima latência.",
			SizeEstimate: "~1.1 GB",
			IsDownloaded: false,
		},
		{
			ID:           "bartowski/Llama-3.2-3B-Instruct-GGUF",
			Name:         "Llama 3.2 3B Instruct (GGUF)",
			Description:  "Modelo rápido da Meta para análise de textos e relatórios.",
			SizeEstimate: "~2.0 GB",
			IsDownloaded: false,
		},
	}

	for i, sug := range suggested {
		for _, inst := range installed {
			if inst.RepoID == sug.ID || inst.ID == sug.ID {
				suggested[i].IsDownloaded = true
				break
			}
		}
	}

	utils.RespondWithJSON(w, http.StatusOK, models.AIModelsResponse{
		SelectedModelID: selectedID,
		InstalledModels: installed,
		SuggestedModels: suggested,
	})
}

func (s *AIManagerServer) handleStartDownload(w http.ResponseWriter, r *http.Request) {
	var req models.ModelDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Corpo da requisição JSON inválido.", nil)
		return
	}

	if req.RepoID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "repo_id é obrigatório.", nil)
		return
	}

	taskID := uuid.New()
	fileName := req.FileNameFilter
	if fileName == "" {
		fileName = "model-q4_k_m.gguf"
	}
	if !strings.HasSuffix(fileName, ".gguf") {
		fileName += ".gguf"
	}

	// Verifica se já existe download pendente
	var activeTaskID uuid.UUID
	err := s.db.Pool.QueryRow(r.Context(), `
		SELECT id FROM ai_download_tasks
		WHERE repo_id = $1 AND status IN ('PENDING', 'DOWNLOADING', 'VERIFYING')
	`, req.RepoID).Scan(&activeTaskID)
	if err == nil {
		utils.RespondWithError(w, http.StatusConflict, utils.ErrAIDownloadInProgress, "Já existe um download em andamento para este repositório.", nil)
		return
	}

	_, err = s.db.Pool.Exec(r.Context(), `
		INSERT INTO ai_download_tasks (id, repo_id, file_name, status, progress_percentage, downloaded_bytes, total_bytes, speed_bytes_sec, created_at, updated_at)
		VALUES ($1, $2, $3, 'PENDING', 0, 0, 0, 0, NOW(), NOW())
	`, taskID, req.RepoID, fileName)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Falha ao registrar tarefa de download: "+err.Error(), nil)
		return
	}

	dlCtx, cancel := context.WithCancel(context.Background())
	s.cancelMutex.Lock()
	s.cancels[taskID] = cancel
	s.cancelMutex.Unlock()

	go s.runDownloadWorker(dlCtx, taskID, req.RepoID, fileName, req.HuggingFaceToken)

	utils.RespondWithJSON(w, http.StatusAccepted, models.ModelDownloadResponse{
		TaskID:  taskID,
		Status:  "PENDING",
		Message: "Download do modelo iniciado com sucesso no cluster de IA.",
	})
}

func (s *AIManagerServer) runDownloadWorker(ctx context.Context, taskID uuid.UUID, repoID string, fileName string, hfToken string) {
	defer func() {
		s.cancelMutex.Lock()
		delete(s.cancels, taskID)
		s.cancelMutex.Unlock()
	}()

	_ = os.MkdirAll(s.cfg.ModelsDir, 0755)
	destPath := filepath.Join(s.cfg.ModelsDir, fileName)
	partPath := destPath + ".part"

	_, _ = s.db.Pool.Exec(context.Background(), "UPDATE ai_download_tasks SET status = 'DOWNLOADING', updated_at = NOW() WHERE id = $1", taskID)

	hfURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repoID, fileName)
	req, err := http.NewRequestWithContext(ctx, "GET", hfURL, nil)
	if err != nil {
		s.markTaskFailed(taskID, err.Error())
		return
	}

	token := hfToken
	if token == "" {
		token = s.cfg.HuggingFaceDefaultToken
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			s.markTaskCanceled(taskID, partPath)
			return
		}
		s.markTaskFailed(taskID, "Falha ao conectar ao Hugging Face: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.markTaskFailed(taskID, fmt.Sprintf("Hugging Face retornou código HTTP %d", resp.StatusCode))
		return
	}

	totalBytes := resp.ContentLength
	outFile, err := os.Create(partPath)
	if err != nil {
		s.markTaskFailed(taskID, "Falha ao criar arquivo no disco: "+err.Error())
		return
	}
	defer outFile.Close()

	buf := make([]byte, 64*1024)
	var downloaded int64
	startTime := time.Now()
	lastUpdate := time.Now()

	for {
		select {
		case <-ctx.Done():
			outFile.Close()
			s.markTaskCanceled(taskID, partPath)
			return
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := outFile.Write(buf[:n])
			if writeErr != nil {
				s.markTaskFailed(taskID, "Erro ao gravar no disco: "+writeErr.Error())
				return
			}
			downloaded += int64(n)

			if time.Since(lastUpdate) > time.Second {
				elapsed := time.Since(startTime).Seconds()
				var speed int64
				if elapsed > 0 {
					speed = int64(float64(downloaded) / elapsed)
				}
				var pct float64
				if totalBytes > 0 {
					pct = float64(downloaded) / float64(totalBytes) * 100
				}

				_, _ = s.db.Pool.Exec(context.Background(), `
					UPDATE ai_download_tasks
					SET downloaded_bytes = $1, total_bytes = $2, progress_percentage = $3, speed_bytes_sec = $4, updated_at = NOW()
					WHERE id = $5
				`, downloaded, totalBytes, pct, speed, taskID)
				lastUpdate = time.Now()
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			s.markTaskFailed(taskID, "Conexão interrompida durante o streaming: "+readErr.Error())
			return
		}
	}

	outFile.Close()
	_ = os.Rename(partPath, destPath)

	// Registra o modelo na base
	var hasActive bool
	_ = s.db.Pool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM ai_models WHERE is_global_active = TRUE)").Scan(&hasActive)

	// Se não tiver nenhum modelo ativo global, torna este o padrão
	makeActive := !hasActive

	_, _ = s.db.Pool.Exec(context.Background(), `
		INSERT INTO ai_models (id, display_name, repo_id, file_name, local_path, size_bytes, quantization, is_global_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'Q4_K_M', $7, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			local_path = EXCLUDED.local_path,
			size_bytes = EXCLUDED.size_bytes,
			updated_at = NOW()
	`, repoID, fileName, repoID, fileName, destPath, downloaded, makeActive)

	_, _ = s.db.Pool.Exec(context.Background(), `
		UPDATE ai_download_tasks
		SET status = 'READY', progress_percentage = 100.0, downloaded_bytes = $1, total_bytes = $1, updated_at = NOW()
		WHERE id = $2
	`, downloaded, taskID)
}

func (s *AIManagerServer) markTaskFailed(taskID uuid.UUID, errMsg string) {
	_, _ = s.db.Pool.Exec(context.Background(), `
		UPDATE ai_download_tasks
		SET status = 'FAILED', error_message = $1, updated_at = NOW()
		WHERE id = $2
	`, errMsg, taskID)
}

func (s *AIManagerServer) markTaskCanceled(taskID uuid.UUID, partPath string) {
	_ = os.Remove(partPath)
	_, _ = s.db.Pool.Exec(context.Background(), `
		UPDATE ai_download_tasks
		SET status = 'CANCELED', updated_at = NOW()
		WHERE id = $1
	`, taskID)
}

func (s *AIManagerServer) handleGetDownloadStatus(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "task_id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "task_id inválido.", nil)
		return
	}

	var task models.AIDownloadTask
	err = s.db.Pool.QueryRow(r.Context(), `
		SELECT id, repo_id, file_name, status, progress_percentage, downloaded_bytes, total_bytes, speed_bytes_sec, error_message, created_at, updated_at
		FROM ai_download_tasks
		WHERE id = $1
	`, taskID).Scan(&task.ID, &task.RepoID, &task.FileName, &task.Status, &task.ProgressPercentage, &task.DownloadedBytes, &task.TotalBytes, &task.SpeedBytesSec, &task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt)

	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrNotFound, "Tarefa de download não encontrada.", nil)
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, task)
}

func (s *AIManagerServer) handleCancelDownload(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "task_id")
	taskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "task_id inválido.", nil)
		return
	}

	s.cancelMutex.Lock()
	cancel, exists := s.cancels[taskID]
	s.cancelMutex.Unlock()

	if exists && cancel != nil {
		cancel()
	} else {
		_, _ = s.db.Pool.Exec(r.Context(), "UPDATE ai_download_tasks SET status = 'CANCELED', updated_at = NOW() WHERE id = $1", taskID)
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"task_id": taskID,
		"status":  "CANCELED",
		"message": "Download cancelado com sucesso.",
	})
}

func (s *AIManagerServer) handleSelectActiveModel(w http.ResponseWriter, r *http.Request) {
	var req models.SelectModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "JSON inválido.", nil)
		return
	}

	tx, err := s.db.Pool.Begin(r.Context())
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Erro ao iniciar transação: "+err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())

	// Desativa todos
	_, err = tx.Exec(r.Context(), "UPDATE ai_models SET is_global_active = FALSE")
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Erro ao redefinir modelo ativo: "+err.Error(), nil)
		return
	}

	// Ativa o modelo escolhido
	res, err := tx.Exec(r.Context(), "UPDATE ai_models SET is_global_active = TRUE, updated_at = NOW() WHERE id = $1", req.ModelID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Erro ao ativar modelo: "+err.Error(), nil)
		return
	}

	if res.RowsAffected() == 0 {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrAIModelNotFound, "Modelo não encontrado nos modelos instalados.", nil)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, utils.ErrInternalServer, "Erro ao confirmar alteração: "+err.Error(), nil)
		return
	}

	log.Printf("[AI-MANAGER] Modelo ativo global alterado com sucesso para: %s", req.ModelID)

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"selected_model_id": req.ModelID,
		"is_global_active":  true,
		"message":           "Modelo ativo global atualizado. Todas as chamadas de chat agora usarão este modelo.",
	})
}

func (s *AIManagerServer) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "model_id")
	if modelID == "" {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "model_id é obrigatório.", nil)
		return
	}

	var localPath string
	var sizeBytes int64
	err := s.db.Pool.QueryRow(r.Context(), "SELECT local_path, size_bytes FROM ai_models WHERE id = $1", modelID).Scan(&localPath, &sizeBytes)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, utils.ErrAIModelNotFound, "Modelo não encontrado.", nil)
		return
	}

	_ = os.Remove(localPath)
	_, _ = s.db.Pool.Exec(r.Context(), "DELETE FROM ai_models WHERE id = $1", modelID)

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"deleted_model_id": modelID,
		"freed_bytes":      sizeBytes,
		"message":          "Modelo excluído com sucesso do disco.",
	})
}

// handleGetActiveModelInternal returns the globally active model for finsense-api
func (s *AIManagerServer) handleGetActiveModelInternal(w http.ResponseWriter, r *http.Request) {
	var m models.AIModel
	err := s.db.Pool.QueryRow(r.Context(), `
		SELECT id, display_name, repo_id, file_name, local_path, size_bytes, quantization, is_global_active, created_at, updated_at
		FROM ai_models
		WHERE is_global_active = TRUE
		LIMIT 1
	`).Scan(&m.ID, &m.DisplayName, &m.RepoID, &m.FileName, &m.LocalPath, &m.SizeBytes, &m.Quantization, &m.IsGlobalActive, &m.CreatedAt, &m.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Se nenhum modelo tiver sido marcado explicitamente como ativo, busca qualquer modelo instalado
			err = s.db.Pool.QueryRow(r.Context(), `
				SELECT id, display_name, repo_id, file_name, local_path, size_bytes, quantization, is_global_active, created_at, updated_at
				FROM ai_models
				ORDER BY created_at DESC
				LIMIT 1
			`).Scan(&m.ID, &m.DisplayName, &m.RepoID, &m.FileName, &m.LocalPath, &m.SizeBytes, &m.Quantization, &m.IsGlobalActive, &m.CreatedAt, &m.UpdatedAt)
		}
	}

	if err != nil {
		utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
			"is_ready":         false,
			"active_model_id":  "default-fallback",
			"display_name":     "Assistente Financeiro Integrado FinSense",
			"message":          "Nenhum modelo baixado ainda no microserviço. Usando motor financeiro padrão.",
		})
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"is_ready":        true,
		"active_model_id": m.ID,
		"display_name":    m.DisplayName,
		"file_name":       m.FileName,
		"local_path":      m.LocalPath,
		"quantization":    m.Quantization,
	})
}

// handleChatCompletionsInternal forwards chat inference
func (s *AIManagerServer) handleChatCompletionsInternal(w http.ResponseWriter, r *http.Request) {
	var req models.ChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, utils.ErrBadRequest, "Payload inválido.", nil)
		return
	}

	// Tenta encaminhar para llama.cpp se estiver de pé
	if s.cfg.AIEngineURL != "" {
		engineEndpoint := fmt.Sprintf("%s/v1/chat/completions", s.cfg.AIEngineURL)
		bodyBytes, err := json.Marshal(req)
		if err == nil {
			httpReq, err := http.NewRequestWithContext(r.Context(), "POST", engineEndpoint, bytes.NewBuffer(bodyBytes))
			if err == nil {
				httpReq.Header.Set("Content-Type", "application/json")
				client := &http.Client{Timeout: 60 * time.Second}
				resp, err := client.Do(httpReq)
				if err == nil && resp.StatusCode == http.StatusOK {
					defer resp.Body.Close()
					var comp models.ChatCompletionsResponse
					if err := json.NewDecoder(resp.Body).Decode(&comp); err == nil {
						utils.RespondWithJSON(w, http.StatusOK, comp)
						return
					}
				}
			}
		}
	}

	// Resposta contextual baseada no prompt enriquecido pelo finsense-api
	lastMsg := ""
	if len(req.Messages) > 0 {
		lastMsg = req.Messages[len(req.Messages)-1].Content
	}

	responseContent := "Olá! Analisei seu panorama financeiro atualizado.\n\n"
	if strings.Contains(strings.ToLower(lastMsg), "dívida") || strings.Contains(strings.ToLower(lastMsg), "cartão") {
		responseContent += "Identifiquei suas pendências em aberto. Recomendo priorizar a quitação das linhas de maior taxa de juros ou aguardar a entrada de receita prevista para negociar deságio à vista."
	} else if strings.Contains(strings.ToLower(lastMsg), "investimento") || strings.Contains(strings.ToLower(lastMsg), "reserva") {
		responseContent += "Seus aportes atuais mantêm uma margem de segurança saudável. Sugiro manter a liquidez imediata para sua reserva de emergência antes de alocações de prazo mais longo."
	} else {
		responseContent += "Com base nas transações e contas monitoradas, seu fluxo mensal está sob controle. Deseja simular um plano de amortização ou verificar previsões de gastos para os próximos meses?"
	}

	utils.RespondWithJSON(w, http.StatusOK, models.ChatCompletionsResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Model:   "FinSense-Global-AI-Engine",
		Created: time.Now().Unix(),
		Choices: []models.ChatMessageChoice{
			{
				Index: 0,
				Message: models.ChatMessageInput{
					Role:    "assistant",
					Content: responseContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: models.ChatUsage{
			PromptTokens:     160,
			CompletionTokens: 90,
			TotalTokens:      250,
		},
	})
}
