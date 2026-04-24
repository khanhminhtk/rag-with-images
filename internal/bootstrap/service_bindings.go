package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	inbound "rag_imagetotext_texttoimage/internal/adapter/inbound"
	inboundRouter "rag_imagetotext_texttoimage/internal/adapter/inbound/router"
	orchestratorUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
	chatUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/chat"
	trainingfile "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/training_file"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
)

type cmdRuntimeBindingKeys struct {
	ConfigKey      string
	LoggerKey      string
	EnvPath        string
	YamlPath       string
	LogLevel       slog.Level
	ResolveLogPath func(*util.Config) string
}

func registerCmdRuntimeBindings(container *DIContainer, keys cmdRuntimeBindingKeys) error {
	if err := registerSingleton(container, keys.ConfigKey, getConfig(keys.EnvPath, keys.YamlPath)); err != nil {
		return fmt.Errorf("register config failed: %w", err)
	}
	if err := registerSingleton(container, keys.LoggerKey, getLoggerFromConfig(keys.ConfigKey, keys.LogLevel, keys.ResolveLogPath)); err != nil {
		return fmt.Errorf("register logger failed: %w", err)
	}
	return nil
}

type orchestratorBindingKeys struct {
	ConfigKey       string
	LoggerKey       string
	HTTPPortKey     string
	ClientsKey      string
	PromptsKey      string
	KafkaInfraKey   string
	SessionTTLKey   string
	SessionStoreKey string
	HTTPServerKey   string
}

func registerOrchestratorBindings(container *DIContainer, keys orchestratorBindingKeys) error {
	if err := registerOrchestratorConfigAndLogger(container, keys); err != nil {
		return err
	}
	if err := registerOrchestratorCoreInfra(container, keys); err != nil {
		return err
	}
	if err := registerOrchestratorHTTPServer(container, keys); err != nil {
		return err
	}
	return nil
}

func registerOrchestratorConfigAndLogger(container *DIContainer, keys orchestratorBindingKeys) error {
	if err := registerSingleton(container, keys.ConfigKey, func(_ Resolver) (any, error) {
		loader := util.NewConfigLoader(ProjectPath("config", ".env"), ProjectPath("config", "config.yaml"))
		cfg, err := loader.Load()
		if err != nil {
			return nil, err
		}
		prepareOrchestratorDefaults(cfg)
		return cfg, nil
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.LoggerKey, getFileLogger(ProjectPath("logs", "orchestrator_service.log"), slog.LevelInfo)); err != nil {
		return err
	}
	return nil
}

func registerOrchestratorCoreInfra(container *DIContainer, keys orchestratorBindingKeys) error {
	if err := registerSingleton(container, keys.HTTPPortKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
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
		return err
	}
	if err := registerSingleton(container, keys.ClientsKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, keys.LoggerKey)
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
		return err
	}
	if err := registerSingleton(container, keys.PromptsKey, func(r Resolver) (any, error) {
		logger, err := ResolveAs[util.Logger](r, keys.LoggerKey)
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
		return err
	}
	if err := registerSingleton(container, keys.KafkaInfraKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, keys.LoggerKey)
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
		return err
	}
	if err := registerSingleton(container, keys.SessionTTLKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
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
		return err
	}
	if err := registerSingleton(container, keys.SessionStoreKey, func(r Resolver) (any, error) {
		ttl, err := ResolveAs[int](r, keys.SessionTTLKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, keys.LoggerKey)
		if err != nil {
			return nil, err
		}
		logger.Info("orchestrator session config", "session_ttl_seconds", ttl)
		return orchestratorUC.NewInMemorySessionStore(time.Duration(ttl) * time.Second), nil
	}); err != nil {
		return err
	}
	return nil
}

func registerOrchestratorHTTPServer(container *DIContainer, keys orchestratorBindingKeys) error {
	if err := registerSingleton(container, keys.HTTPServerKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, keys.LoggerKey)
		if err != nil {
			return nil, err
		}
		httpPort, err := ResolveAs[string](r, keys.HTTPPortKey)
		if err != nil {
			return nil, err
		}
		clients, err := ResolveAs[orchestratorClients](r, keys.ClientsKey)
		if err != nil {
			return nil, err
		}
		prompts, err := ResolveAs[orchestratorPrompts](r, keys.PromptsKey)
		if err != nil {
			return nil, err
		}
		kafkaInfra, err := ResolveAs[orchestratorKafkaInfra](r, keys.KafkaInfraKey)
		if err != nil {
			return nil, err
		}
		sessionStore, err := ResolveAs[*orchestratorUC.InMemorySessionStore](r, keys.SessionStoreKey)
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
		return err
	}
	return nil
}
