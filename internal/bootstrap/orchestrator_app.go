package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	orchestratorUC "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
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

	if err := registerOrchestratorBindings(container, orchestratorBindingKeys{
		ConfigKey:       configKey,
		LoggerKey:       loggerKey,
		HTTPPortKey:     httpPortKey,
		ClientsKey:      clientsKey,
		PromptsKey:      promptsKey,
		KafkaInfraKey:   kafkaInfraKey,
		SessionTTLKey:   sessionTTLKey,
		SessionStoreKey: sessionStoreKey,
		HTTPServerKey:   httpServerKey,
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
