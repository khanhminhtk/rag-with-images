package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	inbound "rag_imagetotext_texttoimage/internal/adapter/inbound"
	inboundRouter "rag_imagetotext_texttoimage/internal/adapter/inbound/router"
	orchestratorUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
	chatUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/chat"
	trainingfile "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/training_file"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
)

func main() {
	configLoader := util.NewConfigLoader("./config/.env", "./config/config.yaml")
	cfg, err := configLoader.Load()
	if err != nil {
		util.Fatalf("failed to load config: %v", err)
	}

	orchestratorPort := strings.TrimSpace(cfg.OrchestratorService.Port)
	if orchestratorPort == "" {
		orchestratorPort = strings.TrimSpace(os.Getenv("ORCHESTRATOR_SERVICE_PORT"))
	}
	if orchestratorPort == "" {
		orchestratorPort = "8080"
	}

	logPath := strings.TrimSpace(cfg.OrchestratorService.LogPath)
	if logPath == "" {
		logPath = "logs/orchestrator_service.log"
	}

	appLogger, err := util.NewFileLogger(logPath, slog.LevelInfo)
	if err != nil {
		util.Fatalf("failed to create logger: %v", err)
	}
	defer appLogger.Close()

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

	prepareOrchestratorDefaults(cfg)

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

	sessionStore := orchestratorUC.NewInMemorySessionStore(time.Duration(sessionTTLSeconds) * time.Second)
	defer sessionStore.Close()
	chatHandlerUC := chatUC.NewChatbotHandler(
		sessionStore,
		*cfg,
		ragClient,
		dlClient,
		llmClient,
		cfg.OrchestratorService.PreProcessing.Model,
		"",
		defaultPromptAnswer,
	)
	vectordbHandlerUC := orchestratorUC.NewVectordbHandler(ragClient)
	trainingFileUseCase := trainingfile.NewTrainingFileUseCase(appLogger, publisher, consumer, *cfg)

	httpHandler := inbound.NewHTTPHandler(
		inboundRouter.NewHTTPHandlerChat(chatHandlerUC),
		inboundRouter.NewHTTPHandlerVectordb(vectordbHandlerUC),
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
			"NewQuery":       "string",
			"CurrentQuery":   "string",
			"CollectionName": "string",
		}
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
