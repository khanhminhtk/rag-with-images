package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	inbound "rag_imagetotext_texttoimage/internal/adapter/inbound"
	inboundRouter "rag_imagetotext_texttoimage/internal/adapter/inbound/router"
	orchestratorUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
	chatUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/chat"
	trainingfile "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/training_file"
	"rag_imagetotext_texttoimage/internal/bootstrap"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
)

func main() {
	cfg, appLogger, err := bootstrap.BuildConfigAndLogger(bootstrap.CmdRuntimeOptions{
		Namespace: "orchestrator_service",
		EnvPath:   "./config/.env",
		YamlPath:  "./config/config.yaml",
		LogLevel:  slog.LevelInfo,
		ResolveLogPath: func(_ *util.Config) string {
			return "logs/orchestrator_service.log"
		},
	})
	if err != nil {
		util.Fatalf("failed to bootstrap orchestrator runtime: %v", err)
	}

	orchestratorPort := strings.TrimSpace(cfg.OrchestratorService.Port)
	if orchestratorPort == "" {
		orchestratorPort = strings.TrimSpace(os.Getenv("ORCHESTRATOR_SERVICE_PORT"))
	}
	if orchestratorPort == "" {
		orchestratorPort = "8080"
	}

	defer appLogger.Close()
	logPath := "logs/orchestrator_service.log"
	appLogger.Info("orchestrator bootstrap started", "http_port", orchestratorPort, "log_path", logPath)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ragHost := strings.TrimSpace(cfg.RAGService.GRPCHost)
	if ragHost == "" {
		ragHost = "localhost"
	}
	ragPort := strings.TrimSpace(cfg.RAGService.GRPCPort)
	if ragPort == "" {
		ragPort = strings.TrimSpace(cfg.RAGService.Port)
	}
	ragClient, ragConn, err := orchestratorUC.NewRagServiceClient(ctx, ragHost, ragPort)
	if err != nil {
		util.Fatalf("failed to create rag service client: %v", err)
	}
	defer ragConn.Close()
	appLogger.Info("orchestrator dependency ready", "service", "rag", "host", ragHost, "port", ragPort)

	llmHost := strings.TrimSpace(os.Getenv("LLM_SERVICE_HOST"))
	if llmHost == "" {
		llmHost = "localhost"
	}
	llmPort := strings.TrimSpace(cfg.LLMService.Port)
	llmClient, llmConn, err := orchestratorUC.NewLLMServiceClient(ctx, llmHost, llmPort)
	if err != nil {
		util.Fatalf("failed to create llm service client: %v", err)
	}
	defer llmConn.Close()
	appLogger.Info("orchestrator dependency ready", "service", "llm", "host", llmHost, "port", llmPort)

	dlHost := strings.TrimSpace(os.Getenv("EMBEDDING_SERVICE_HOST"))
	if dlHost == "" {
		dlHost = "localhost"
	}
	dlPort := strings.TrimSpace(cfg.EmbeddingService.Port)
	dlClient, dlConn, err := orchestratorUC.NewDeepLearningServiceClient(ctx, dlHost, dlPort)
	if err != nil {
		util.Fatalf("failed to create embedding service client: %v", err)
	}
	defer dlConn.Close()
	appLogger.Info("orchestrator dependency ready", "service", "embedding", "host", dlHost, "port", dlPort)

	prepareOrchestratorDefaults(cfg)
	promptPreprocessing, prePromptPath, err := loadPromptFromCandidates(promptPreprocessingCandidates)
	if err != nil {
		util.Fatalf("failed to load preprocessing prompt: %v", err)
	}
	promptPostprocessing, postPromptPath, err := loadPromptFromCandidates(promptPostprocessingCandidates)
	if err != nil {
		util.Fatalf("failed to load postprocessing prompt: %v", err)
	}
	appLogger.Info(
		"orchestrator prompts loaded",
		"preprocessing_path", prePromptPath,
		"preprocessing_chars", len(promptPreprocessing),
		"postprocessing_path", postPromptPath,
		"postprocessing_chars", len(promptPostprocessing),
	)

	kafkaClient, err := infraKafka.NewKafkaClient(infraKafka.KafkaConfig{
		Brokers:     normalizeBrokers(cfg.Kafka.Brokers),
		DialTimeout: 10 * time.Second,
	}, appLogger)
	if err != nil {
		util.Fatalf("failed to create kafka client: %v", err)
	}

	publisher, err := infraKafka.NewPublisher(kafkaClient, infraKafka.PublisherConfig{
		RequiredAcks: -1,
	}, appLogger)
	if err != nil {
		util.Fatalf("failed to create kafka publisher: %v", err)
	}
	defer publisher.Close()

	consumer, err := infraKafka.NewKafkaConsumer(kafkaClient, infraKafka.ConsumerConfig{
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    -2,
	}, appLogger)
	if err != nil {
		util.Fatalf("failed to create kafka consumer: %v", err)
	}
	defer consumer.Close()

	sessionTTLSeconds := cfg.OrchestratorService.SessionTTLSeconds
	if sessionTTLSeconds <= 0 {
		if raw := strings.TrimSpace(os.Getenv("ORCHESTRATOR_SERVICE_SESSION_TTL_SECONDS")); raw != "" {
			if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
				sessionTTLSeconds = parsed
			}
		}
	}
	if sessionTTLSeconds <= 0 {
		sessionTTLSeconds = 1800
	}
	appLogger.Info("orchestrator session config", "session_ttl_seconds", sessionTTLSeconds)

	sessionStore := orchestratorUC.NewInMemorySessionStore(time.Duration(sessionTTLSeconds) * time.Second)
	defer sessionStore.Close()
	chatHandlerUC := chatUC.NewChatbotHandler(
		sessionStore,
		appLogger,
		*cfg,
		ragClient,
		dlClient,
		llmClient,
		promptPreprocessing,
		promptPostprocessing,
		defaultPromptAnswer,
	)
	vectordbHandlerUC := orchestratorUC.NewVectordbHandler(ragClient)
	trainingFileUseCase := trainingfile.NewTrainingFileUseCase(appLogger, publisher, consumer, *cfg)

	httpHandler := inbound.NewHTTPHandler(
		inboundRouter.NewHTTPHandlerChat(chatHandlerUC),
		inboundRouter.NewHTTPHandlerVectordb(vectordbHandlerUC, cfg.OrchestratorService.Vectordb),
		inboundRouter.NewHTTPHandlerTrainingFile(trainingFileUseCase),
	)
	router := inbound.SetupRouter(httpHandler)
	server := inbound.NewHTTPServer(":"+orchestratorPort, router)

	go func() {
		appLogger.Info("orchestrator http server listening", "port", orchestratorPort)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			appLogger.Error("orchestrator http server failed", serveErr)
			util.Fatalf("failed to serve orchestrator http: %v", serveErr)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	appLogger.Info("orchestrator graceful shutdown start")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("orchestrator graceful shutdown failed", err)
	}
	appLogger.Info("orchestrator service stopped")
}

func prepareOrchestratorDefaults(cfg *util.Config) {
	if cfg == nil {
		return
	}

	if strings.TrimSpace(cfg.OrchestratorService.PreProcessing.Model) == "" {
		cfg.OrchestratorService.PreProcessing.Model = strings.TrimSpace(cfg.LLMService.Model)
	}
	if cfg.OrchestratorService.PreProcessing.Temperature == 0 {
		cfg.OrchestratorService.PreProcessing.Temperature = cfg.LLMService.Temp
	}
	if cfg.OrchestratorService.PreProcessing.StructOutput == nil {
		cfg.OrchestratorService.PreProcessing.StructOutput = map[string]string{
			"NewQuery":     "string",
			"CurrentQuery": "string",
		}
	}
	if cfg.OrchestratorService.MemoryHistoryTopK <= 0 {
		cfg.OrchestratorService.MemoryHistoryTopK = 5
	}
	if cfg.OrchestratorService.Vectordb.Shards == 0 {
		cfg.OrchestratorService.Vectordb.Shards = 1
	}
	if cfg.OrchestratorService.Vectordb.ReplicationFactor == 0 {
		cfg.OrchestratorService.Vectordb.ReplicationFactor = 1
	}
	if cfg.OrchestratorService.Vectordb.TextVectorSize == 0 {
		cfg.OrchestratorService.Vectordb.TextVectorSize = 768
	}
	if cfg.OrchestratorService.Vectordb.ImageVectorSize == 0 {
		cfg.OrchestratorService.Vectordb.ImageVectorSize = 768
	}
	if strings.TrimSpace(cfg.OrchestratorService.Vectordb.TextVectorDistance) == "" {
		cfg.OrchestratorService.Vectordb.TextVectorDistance = "cosine"
	}
	if strings.TrimSpace(cfg.OrchestratorService.Vectordb.ImageVectorDistance) == "" {
		cfg.OrchestratorService.Vectordb.ImageVectorDistance = "cosine"
	}
}

func normalizeBrokers(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, broker := range strings.Split(item, ",") {
			broker = strings.TrimSpace(broker)
			if broker == "" {
				continue
			}
			out = append(out, broker)
		}
	}
	if len(out) == 0 {
		return raw
	}
	return out
}

const defaultPromptAnswer = "Dua tren context duoc cung cap, hay tra loi cau hoi mot cach ngan gon, chinh xac va bang tieng Viet."

var promptPreprocessingCandidates = []string{
	"./data/prompt/preprocessing.txt",
	"../../data/prompt/preprocessing.txt",
}

var promptPostprocessingCandidates = []string{
	"./data/prompt/postprocessing.txt",
	"../../data/prompt/postprocessing.txt",
}

func loadPromptFromCandidates(candidates []string) (string, string, error) {
	for _, candidate := range candidates {
		resolved := filepath.Clean(strings.TrimSpace(candidate))
		if resolved == "" {
			continue
		}
		raw, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		prompt := strings.TrimSpace(string(raw))
		if prompt == "" {
			return "", resolved, fmt.Errorf("prompt file is empty: %s", resolved)
		}
		return prompt, resolved, nil
	}
	return "", "", fmt.Errorf("prompt file not found in candidates: %s", strings.Join(candidates, ", "))
}
