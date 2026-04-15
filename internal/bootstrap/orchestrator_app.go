package bootstrap

import (
	"context"
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

	"google.golang.org/grpc"

	inbound "rag_imagetotext_texttoimage/internal/adapter/inbound"
	inboundRouter "rag_imagetotext_texttoimage/internal/adapter/inbound/router"
	orchestratorUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
	chatUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/chat"
	trainingfile "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/training_file"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type OrchestratorApp struct {
	cfg        *util.Config
	logger     util.Logger
	httpPort   string
	httpServer *http.Server
	session    *orchestratorUC.InMemorySessionStore
	publisher  *infraKafka.Publisher
	consumer   *infraKafka.Consumer
	ragConn    *grpc.ClientConn
	llmConn    *grpc.ClientConn
	dlConn     *grpc.ClientConn
}

type orchestratorClients struct {
	ragClient pb.RagServiceClient
	ragConn   *grpc.ClientConn
	llmClient pb.LlmServiceClient
	llmConn   *grpc.ClientConn
	dlClient  pb.DeepLearningServiceClient
	dlConn    *grpc.ClientConn
}

type orchestratorPrompts struct {
	preprocessing  string
	postprocessing string
}

type orchestratorKafkaInfra struct {
	publisher *infraKafka.Publisher
	consumer  *infraKafka.Consumer
}

func NewOrchestratorApp() (*OrchestratorApp, error) {
	container := NewContainer()
	registry := NewRegistry(container, "orchestrator_service")

	configKey := registry.Key("config")
	loggerKey := registry.Key("logger")
	httpPortKey := registry.Key("http.port")
	clientsKey := registry.Key("upstream.clients")
	promptsKey := registry.Key("prompts")
	kafkaInfraKey := registry.Key("kafka.infra")
	sessionTTLKey := registry.Key("session.ttl_seconds")
	sessionStoreKey := registry.Key("session.store")
	httpServerKey := registry.Key("http.server")

	if err := registry.RegisterSingleton(configKey, func(_ Resolver) (any, error) {
		loader := util.NewConfigLoader(ProjectPath("config", ".env"), ProjectPath("config", "config.yaml"))
		cfg, err := loader.Load()
		if err != nil {
			return nil, err
		}
		prepareOrchestratorDefaults(cfg)
		return cfg, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(loggerKey, func(_ Resolver) (any, error) {
		return util.NewFileLogger(ProjectPath("logs", "orchestrator_service.log"), slog.LevelInfo)
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(httpPortKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		port := strings.TrimSpace(cfg.OrchestratorService.Port)
		if port == "" {
			port = strings.TrimSpace(os.Getenv("ORCHESTRATOR_SERVICE_PORT"))
		}
		if port == "" {
			port = "8080"
		}
		return port, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(clientsKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
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
			return nil, fmt.Errorf("create rag service client: %w", err)
		}
		logger.Info("orchestrator dependency ready", "service", "rag", "host", ragHost, "port", ragPort)

		llmHost := strings.TrimSpace(os.Getenv("LLM_SERVICE_HOST"))
		if llmHost == "" {
			llmHost = "localhost"
		}
		llmPort := strings.TrimSpace(cfg.LLMService.Port)
		llmClient, llmConn, err := orchestratorUC.NewLLMServiceClient(ctx, llmHost, llmPort)
		if err != nil {
			_ = ragConn.Close()
			return nil, fmt.Errorf("create llm service client: %w", err)
		}
		logger.Info("orchestrator dependency ready", "service", "llm", "host", llmHost, "port", llmPort)

		dlHost := strings.TrimSpace(os.Getenv("EMBEDDING_SERVICE_HOST"))
		if dlHost == "" {
			dlHost = "localhost"
		}
		dlPort := strings.TrimSpace(cfg.EmbeddingService.Port)
		dlClient, dlConn, err := orchestratorUC.NewDeepLearningServiceClient(ctx, dlHost, dlPort)
		if err != nil {
			_ = ragConn.Close()
			_ = llmConn.Close()
			return nil, fmt.Errorf("create embedding service client: %w", err)
		}
		logger.Info("orchestrator dependency ready", "service", "embedding", "host", dlHost, "port", dlPort)

		return orchestratorClients{
			ragClient: ragClient,
			ragConn:   ragConn,
			llmClient: llmClient,
			llmConn:   llmConn,
			dlClient:  dlClient,
			dlConn:    dlConn,
		}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(promptsKey, func(r Resolver) (any, error) {
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		pre, prePath, err := loadPromptFromCandidates(promptPreprocessingCandidates)
		if err != nil {
			return nil, fmt.Errorf("load preprocessing prompt: %w", err)
		}
		post, postPath, err := loadPromptFromCandidates(promptPostprocessingCandidates)
		if err != nil {
			return nil, fmt.Errorf("load postprocessing prompt: %w", err)
		}
		logger.Info("orchestrator prompts loaded", "preprocessing_path", prePath, "preprocessing_chars", len(pre), "postprocessing_path", postPath, "postprocessing_chars", len(post))
		return orchestratorPrompts{preprocessing: pre, postprocessing: post}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(kafkaInfraKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		kafkaClient, err := infraKafka.NewKafkaClient(infraKafka.KafkaConfig{Brokers: normalizeBrokers(cfg.Kafka.Brokers), DialTimeout: 10 * time.Second}, logger)
		if err != nil {
			return nil, fmt.Errorf("create kafka client: %w", err)
		}
		publisher, err := infraKafka.NewPublisher(kafkaClient, infraKafka.PublisherConfig{RequiredAcks: -1}, logger)
		if err != nil {
			return nil, fmt.Errorf("create kafka publisher: %w", err)
		}
		consumer, err := infraKafka.NewKafkaConsumer(kafkaClient, infraKafka.ConsumerConfig{MinBytes: 1, MaxBytes: 10e6, CommitInterval: time.Second, StartOffset: -2}, logger)
		if err != nil {
			_ = publisher.Close()
			return nil, fmt.Errorf("create kafka consumer: %w", err)
		}
		return orchestratorKafkaInfra{publisher: publisher, consumer: consumer}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(sessionTTLKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		ttl := cfg.OrchestratorService.SessionTTLSeconds
		if ttl <= 0 {
			if raw := strings.TrimSpace(os.Getenv("ORCHESTRATOR_SERVICE_SESSION_TTL_SECONDS")); raw != "" {
				if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
					ttl = parsed
				}
			}
		}
		if ttl <= 0 {
			ttl = 1800
		}
		return ttl, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(sessionStoreKey, func(r Resolver) (any, error) {
		ttl, err := ResolveAs[int](r, sessionTTLKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		logger.Info("orchestrator session config", "session_ttl_seconds", ttl)
		return orchestratorUC.NewInMemorySessionStore(time.Duration(ttl) * time.Second), nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(httpServerKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		httpPort, err := ResolveAs[string](r, httpPortKey)
		if err != nil {
			return nil, err
		}
		clients, err := ResolveAs[orchestratorClients](r, clientsKey)
		if err != nil {
			return nil, err
		}
		prompts, err := ResolveAs[orchestratorPrompts](r, promptsKey)
		if err != nil {
			return nil, err
		}
		kafkaInfra, err := ResolveAs[orchestratorKafkaInfra](r, kafkaInfraKey)
		if err != nil {
			return nil, err
		}
		sessionStore, err := ResolveAs[*orchestratorUC.InMemorySessionStore](r, sessionStoreKey)
		if err != nil {
			return nil, err
		}

		chatHandlerUC := chatUC.NewChatbotHandler(sessionStore, logger, *cfg, clients.ragClient, clients.dlClient, clients.llmClient, prompts.preprocessing, prompts.postprocessing, defaultPromptAnswer)
		vectordbHandlerUC := orchestratorUC.NewVectordbHandler(clients.ragClient)
		trainingFileUseCase := trainingfile.NewTrainingFileUseCase(logger, kafkaInfra.publisher, kafkaInfra.consumer, *cfg)
		httpHandler := inbound.NewHTTPHandler(
			inboundRouter.NewHTTPHandlerChat(chatHandlerUC),
			inboundRouter.NewHTTPHandlerVectordb(vectordbHandlerUC, cfg.OrchestratorService.Vectordb),
			inboundRouter.NewHTTPHandlerTrainingFile(trainingFileUseCase),
		)
		router := inbound.SetupRouter(httpHandler)
		return inbound.NewHTTPServer(":"+httpPort, router), nil
	}); err != nil {
		return nil, err
	}

	cfg, err := ResolveAs[*util.Config](container, configKey)
	if err != nil {
		return nil, err
	}
	logger, err := ResolveAs[util.Logger](container, loggerKey)
	if err != nil {
		return nil, err
	}
	httpPort, err := ResolveAs[string](container, httpPortKey)
	if err != nil {
		logger.Close()
		return nil, err
	}
	clients, err := ResolveAs[orchestratorClients](container, clientsKey)
	if err != nil {
		logger.Close()
		return nil, err
	}
	kafkaInfra, err := ResolveAs[orchestratorKafkaInfra](container, kafkaInfraKey)
	if err != nil {
		_ = clients.ragConn.Close()
		_ = clients.llmConn.Close()
		_ = clients.dlConn.Close()
		logger.Close()
		return nil, err
	}
	sessionStore, err := ResolveAs[*orchestratorUC.InMemorySessionStore](container, sessionStoreKey)
	if err != nil {
		_ = kafkaInfra.publisher.Close()
		_ = kafkaInfra.consumer.Close()
		_ = clients.ragConn.Close()
		_ = clients.llmConn.Close()
		_ = clients.dlConn.Close()
		logger.Close()
		return nil, err
	}
	httpServer, err := ResolveAs[*http.Server](container, httpServerKey)
	if err != nil {
		sessionStore.Close()
		_ = kafkaInfra.publisher.Close()
		_ = kafkaInfra.consumer.Close()
		_ = clients.ragConn.Close()
		_ = clients.llmConn.Close()
		_ = clients.dlConn.Close()
		logger.Close()
		return nil, err
	}

	logger.Info("orchestrator bootstrap started", "http_port", httpPort, "log_path", ProjectPath("logs", "orchestrator_service.log"))

	return &OrchestratorApp{
		cfg:        cfg,
		logger:     logger,
		httpPort:   httpPort,
		httpServer: httpServer,
		session:    sessionStore,
		publisher:  kafkaInfra.publisher,
		consumer:   kafkaInfra.consumer,
		ragConn:    clients.ragConn,
		llmConn:    clients.llmConn,
		dlConn:     clients.dlConn,
	}, nil
}

func (a *OrchestratorApp) Run() error {
	if a == nil {
		return fmt.Errorf("internal.bootstrap.OrchestratorApp.Run app is nil")
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("orchestrator http server listening", "port", a.httpPort)
		if serveErr := a.httpServer.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
		close(errCh)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signalCh:
		signal.Stop(signalCh)
		close(signalCh)
		a.logger.Info("orchestrator graceful shutdown start")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("orchestrator graceful shutdown failed", err)
		}
		a.logger.Info("orchestrator service stopped")
		return nil
	case err := <-errCh:
		if err != nil {
			a.logger.Error("orchestrator http server failed", err)
			return err
		}
		return nil
	}
}

func (a *OrchestratorApp) Close() {
	if a == nil {
		return
	}
	if a.session != nil {
		a.session.Close()
	}
	if a.publisher != nil {
		_ = a.publisher.Close()
	}
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.ragConn != nil {
		_ = a.ragConn.Close()
	}
	if a.llmConn != nil {
		_ = a.llmConn.Close()
	}
	if a.dlConn != nil {
		_ = a.dlConn.Close()
	}
	if a.logger != nil {
		a.logger.Close()
	}
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
		cfg.OrchestratorService.PreProcessing.StructOutput = map[string]string{"NewQuery": "string", "CurrentQuery": "string"}
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
	ProjectPath("data", "prompt", "preprocessing.txt"),
	"./data/prompt/preprocessing.txt",
	"../../data/prompt/preprocessing.txt",
}

var promptPostprocessingCandidates = []string{
	ProjectPath("data", "prompt", "postprocessing.txt"),
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
