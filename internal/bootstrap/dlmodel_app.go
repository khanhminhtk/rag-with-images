package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/cgo"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type DLModelApp struct {
	cfg        *util.Config
	logger     util.Logger
	jinaConfig *dlmodelJinaRuntimeConfig
	jina       *cgo.JinaAdapter
	producer   *kafkaAdapter.ProducerAdapter
	consumer   *kafkaAdapter.ConsumerAdapter
	topics     util.EmbeddingTopics
	grpcServer *grpc.Server
	listener   net.Listener
}

type dlmodelKafkaInfra struct {
	producer *kafkaAdapter.ProducerAdapter
	consumer *kafkaAdapter.ConsumerAdapter
}

func NewDLModelApp() (*DLModelApp, error) {
	container := NewContainer()
	registry := NewRegistry(container, "dlmodel_service")

	configKey := registry.Key("config")
	loggerKey := registry.Key("logger")
	jinaRuntimeKey := registry.Key("jina.runtime_config")
	jinaAdapterKey := registry.Key("jina.adapter")
	topicsKey := registry.Key("topics")
	kafkaInfraKey := registry.Key("kafka.infra")
	grpcServerKey := registry.Key("grpc.server")
	listenerKey := registry.Key("grpc.listener")

	if err := registry.RegisterSingleton(configKey, func(_ Resolver) (any, error) {
		loader := util.NewConfigLoader(ProjectPath("config", ".env"), ProjectPath("config", "config.yaml"))
		return loader.Load()
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(loggerKey, func(r Resolver) (any, error) {
		return util.NewFileLogger(ProjectPath("logs", "embedding_service.log"), slog.LevelInfo)
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(jinaRuntimeKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		resolvedJinaConfigPath, err := resolveJinaConfigPath(strings.TrimSpace(cfg.EmbeddingService.JinaConfigPath))
		if err != nil {
			logger.Error("failed to resolve jina config path", err, "config_path", cfg.EmbeddingService.JinaConfigPath)
			return nil, err
		}
		runtimeConfigPath, cleanup, err := prepareJinaRuntime(resolvedJinaConfigPath)
		if err != nil {
			logger.Error("failed to prepare jina runtime", err, "resolved_config_path", resolvedJinaConfigPath)
			return nil, err
		}
		if err := validateJinaRuntimeConfig(runtimeConfigPath); err != nil {
			cleanup()
			logger.Error("invalid jina runtime config", err, "config_path", runtimeConfigPath)
			return nil, err
		}
		return &dlmodelJinaRuntimeConfig{path: runtimeConfigPath, cleanup: cleanup}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(jinaAdapterKey, func(r Resolver) (any, error) {
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		runtimeCfg, err := ResolveAs[*dlmodelJinaRuntimeConfig](r, jinaRuntimeKey)
		if err != nil {
			return nil, err
		}
		jina, err := cgo.NewJinaAdapter(runtimeCfg.path, logger)
		if err != nil {
			return nil, err
		}
		logger.Info("jina adapter ready", "runtime_config_path", runtimeCfg.path)
		return jina, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(topicsKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		return buildEmbeddingTopics(cfg.EmbeddingService.Topics)
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
		producer, consumer, err := kafkaAdapter.NewInfraAdapters(kafkaAdapter.InfraAdapterConfig{
			Brokers:     cfg.Kafka.Brokers,
			DialTimeout: 10 * time.Second,
			Publisher:   infraKafka.PublisherConfig{RequiredAcks: -1},
			Consumer: infraKafka.ConsumerConfig{
				MinBytes:       1,
				MaxBytes:       10e6,
				CommitInterval: time.Second,
				StartOffset:    -2,
			},
		}, logger)
		if err != nil {
			return nil, err
		}
		return dlmodelKafkaInfra{producer: producer, consumer: consumer}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(grpcServerKey, func(r Resolver) (any, error) {
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		jina, err := ResolveAs[*cgo.JinaAdapter](r, jinaAdapterKey)
		if err != nil {
			return nil, err
		}
		embeddingService := grpcAdapter.NewEmbeddingService(logger, jina)
		grpcServer := grpc.NewServer(
			grpc.MaxRecvMsgSize(30*1024*1024),
			grpc.MaxSendMsgSize(30*1024*1024),
		)
		pb.RegisterDeepLearningServiceServer(grpcServer, embeddingService)
		reflection.Register(grpcServer)
		logger.Info("embedding grpc reflection enabled")
		return grpcServer, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(listenerKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.EmbeddingService.Port))
		if err != nil {
			logger.Error("failed to listen grpc port", err, "port", cfg.EmbeddingService.Port)
			return nil, err
		}
		return lis, nil
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
	jinaCfg, err := ResolveAs[*dlmodelJinaRuntimeConfig](container, jinaRuntimeKey)
	if err != nil {
		logger.Close()
		return nil, err
	}
	jina, err := ResolveAs[*cgo.JinaAdapter](container, jinaAdapterKey)
	if err != nil {
		jinaCfg.cleanup()
		logger.Close()
		return nil, err
	}
	topics, err := ResolveAs[util.EmbeddingTopics](container, topicsKey)
	if err != nil {
		jina.Close()
		jinaCfg.cleanup()
		logger.Close()
		return nil, err
	}
	kafkaInfra, err := ResolveAs[dlmodelKafkaInfra](container, kafkaInfraKey)
	if err != nil {
		jina.Close()
		jinaCfg.cleanup()
		logger.Close()
		return nil, err
	}
	grpcServer, err := ResolveAs[*grpc.Server](container, grpcServerKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		jina.Close()
		jinaCfg.cleanup()
		logger.Close()
		return nil, err
	}
	lis, err := ResolveAs[net.Listener](container, listenerKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		jina.Close()
		jinaCfg.cleanup()
		logger.Close()
		return nil, err
	}

	logger.Info("embedding service bootstrap started", "grpc_port", cfg.EmbeddingService.Port, "log_path", ProjectPath("logs", "embedding_service.log"), "jina_config_path", cfg.EmbeddingService.JinaConfigPath)

	return &DLModelApp{
		cfg:        cfg,
		logger:     logger,
		jinaConfig: jinaCfg,
		jina:       jina,
		producer:   kafkaInfra.producer,
		consumer:   kafkaInfra.consumer,
		topics:     topics,
		grpcServer: grpcServer,
		listener:   lis,
	}, nil
}

func (a *DLModelApp) Run() error {
	if a == nil {
		return fmt.Errorf("internal.bootstrap.DLModelApp.Run app is nil")
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	batchTextErrCh := a.consumer.Start(rootCtx, a.topics.BatchTextRequest, a.topics.BatchTextGroup, a.handleBatchText)
	batchImageErrCh := a.consumer.Start(rootCtx, a.topics.BatchImageRequest, a.topics.BatchImageGroup, a.handleBatchImage)

	grpcErrCh := make(chan error, 1)
	go func() {
		a.logger.Info("embedding grpc server listening", "port", a.cfg.EmbeddingService.Port)
		if serveErr := a.grpcServer.Serve(a.listener); serveErr != nil {
			grpcErrCh <- serveErr
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalCh:
		a.logger.Info("received shutdown signal")
	case err := <-grpcErrCh:
		if err != nil {
			a.logger.Error("grpc server stopped with error", err)
		}
	case err := <-batchTextErrCh:
		if err != nil {
			a.logger.Error("batch text consumer stopped with error", err)
		}
	case err := <-batchImageErrCh:
		if err != nil {
			a.logger.Error("batch image consumer stopped with error", err)
		}
	}

	signal.Stop(signalCh)
	close(signalCh)
	cancel()
	waitConsumerStopped(batchTextErrCh, "batch_text", a.logger, 15*time.Second)
	waitConsumerStopped(batchImageErrCh, "batch_image", a.logger, 15*time.Second)
	a.grpcServer.GracefulStop()
	a.logger.Info("service stopped")
	return nil
}

func (a *DLModelApp) Close() {
	if a == nil {
		return
	}
	if a.producer != nil {
		_ = a.producer.Close()
	}
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.jina != nil {
		a.jina.Close()
	}
	if a.jinaConfig != nil && a.jinaConfig.cleanup != nil {
		a.jinaConfig.cleanup()
	}
	if a.logger != nil {
		a.logger.Close()
	}
}

func buildEmbeddingTopics(raw util.EmbeddingTopics) (util.EmbeddingTopics, error) {
	topics := raw
	topics.BatchTextRequest = strings.TrimSpace(topics.BatchTextRequest)
	topics.BatchTextGroup = strings.TrimSpace(topics.BatchTextGroup)
	topics.BatchTextResult = strings.TrimSpace(topics.BatchTextResult)
	topics.BatchImageRequest = strings.TrimSpace(topics.BatchImageRequest)
	topics.BatchImageGroup = strings.TrimSpace(topics.BatchImageGroup)
	topics.BatchImageResult = strings.TrimSpace(topics.BatchImageResult)
	if topics.BatchTextRequest == "" || topics.BatchTextGroup == "" {
		return util.EmbeddingTopics{}, fmt.Errorf("embedding batch text topic/group is empty")
	}
	if topics.BatchImageRequest == "" || topics.BatchImageGroup == "" {
		return util.EmbeddingTopics{}, fmt.Errorf("embedding batch image topic/group is empty")
	}
	return topics, nil
}

func (a *DLModelApp) handleBatchText(ctx context.Context, msg ports.ConsumeMessage) error {
	startedAt := time.Now()
	var request struct {
		Texts []string `json:"texts"`
	}
	if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
		a.logger.Error("invalid batch text payload", err, "topic", msg.Topic, "offset", msg.Offset)
		return nil
	}
	embeddings, runErr := a.jina.EmbedBatchText(request.Texts)
	status := "success"
	message := "batch text embedding completed"
	dimension := 0
	if len(embeddings) > 0 {
		dimension = len(embeddings[0])
	}
	if runErr != nil {
		status = "failed"
		message = runErr.Error()
		a.logger.Error("batch text embedding failed", runErr, "topic", msg.Topic, "offset", msg.Offset)
	}
	if a.topics.BatchTextResult != "" {
		publishErr := a.producer.PublishJSON(ctx, a.topics.BatchTextResult, msg.Message.Key, map[string]any{
			"status":     status,
			"message":    message,
			"count":      len(request.Texts),
			"dimension":  dimension,
			"embeddings": embeddings,
			"at":         time.Now().UTC().Format(time.RFC3339),
		}, map[string]string{"source_topic": a.topics.BatchTextRequest})
		if publishErr != nil {
			a.logger.Error("publish batch text result failed", publishErr, "result_topic", a.topics.BatchTextResult)
		}
	}
	a.logger.Info("batch text pipeline completed", "topic", msg.Topic, "status", status, "count", len(request.Texts), "dimension", dimension, "latency_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func (a *DLModelApp) handleBatchImage(ctx context.Context, msg ports.ConsumeMessage) error {
	startedAt := time.Now()
	var request struct {
		Images   [][]byte `json:"images"`
		Width    int      `json:"width"`
		Height   int      `json:"height"`
		Channels int      `json:"channels"`
	}
	if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
		a.logger.Error("invalid batch image payload", err, "topic", msg.Topic, "offset", msg.Offset)
		return nil
	}
	embeddings, runErr := a.jina.EmbedBatchImage(request.Images, request.Width, request.Height, request.Channels)
	status := "success"
	message := "batch image embedding completed"
	dimension := 0
	if len(embeddings) > 0 {
		dimension = len(embeddings[0])
	}
	if runErr != nil {
		status = "failed"
		message = runErr.Error()
		a.logger.Error("batch image embedding failed", runErr, "topic", msg.Topic, "offset", msg.Offset)
	}
	if a.topics.BatchImageResult != "" {
		publishErr := a.producer.PublishJSON(ctx, a.topics.BatchImageResult, msg.Message.Key, map[string]any{
			"status":     status,
			"message":    message,
			"count":      len(request.Images),
			"dimension":  dimension,
			"embeddings": embeddings,
			"at":         time.Now().UTC().Format(time.RFC3339),
		}, map[string]string{"source_topic": a.topics.BatchImageRequest})
		if publishErr != nil {
			a.logger.Error("publish batch image result failed", publishErr, "result_topic", a.topics.BatchImageResult)
		}
	}
	a.logger.Info("batch image pipeline completed", "topic", msg.Topic, "status", status, "count", len(request.Images), "dimension", dimension, "latency_ms", time.Since(startedAt).Milliseconds())
	return nil
}
