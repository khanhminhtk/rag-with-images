package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/cgo"
	"rag_imagetotext_texttoimage/internal/infra/monitoring"
	"rag_imagetotext_texttoimage/internal/util"
)

type DLModelApp struct {
	cfg          *util.Config
	logger       util.Logger
	jinaConfig   *dlmodelJinaRuntimeConfig
	jina         *cgo.JinaAdapter
	producer     *kafkaAdapter.ProducerAdapter
	consumer     *kafkaAdapter.ConsumerAdapter
	topics       util.EmbeddingTopics
	grpcServer   *grpc.Server
	listener     net.Listener
	metricsSrv   *http.Server
	metricsKafka *monitoring.Metrics
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
	metricsHTTPKey := registry.Key("metrics.http_server")

	if err := registerDLModelBindings(container, dlmodelBindingKeys{
		ConfigKey:      configKey,
		LoggerKey:      loggerKey,
		JinaRuntimeKey: jinaRuntimeKey,
		JinaAdapterKey: jinaAdapterKey,
		TopicsKey:      topicsKey,
		KafkaInfraKey:  kafkaInfraKey,
		GRPCServerKey:  grpcServerKey,
		ListenerKey:    listenerKey,
		MetricsHTTPKey: metricsHTTPKey,
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
	metricsSrv, err := ResolveAs[*http.Server](container, metricsHTTPKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		jina.Close()
		jinaCfg.cleanup()
		logger.Close()
		return nil, err
	}

	logger.Info("dlmodel service bootstrap started", "grpc_port", cfg.EmbeddingService.Port, "log_path", ProjectPath("logs", "dlmodel_service.log"), "jina_config_path", cfg.EmbeddingService.JinaConfigPath)

	return &DLModelApp{
		cfg:          cfg,
		logger:       logger,
		jinaConfig:   jinaCfg,
		jina:         jina,
		producer:     kafkaInfra.producer,
		consumer:     kafkaInfra.consumer,
		topics:       topics,
		grpcServer:   grpcServer,
		listener:     lis,
		metricsSrv:   metricsSrv,
		metricsKafka: monitoring.NewMetrics(),
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
		a.logger.Info("dlmodel service grpc server listening", "port", a.cfg.EmbeddingService.Port)
		if serveErr := a.grpcServer.Serve(a.listener); serveErr != nil {
			grpcErrCh <- serveErr
		}
	}()

	metricsErrCh := make(chan error, 1)
	go func() {
		if a.metricsSrv == nil {
			return
		}
		a.logger.Info("dlmodel service metrics server listening", "addr", a.metricsSrv.Addr)
		if serveErr := a.metricsSrv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			metricsErrCh <- serveErr
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
	case err := <-metricsErrCh:
		if err != nil {
			a.logger.Error("metrics server stopped with error", err)
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
	if a.metricsSrv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.metricsSrv.Shutdown(shutdownCtx)
		shutdownCancel()
	}
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

func (a *DLModelApp) handleBatchText(ctx context.Context, msg ports.ConsumeMessage) (handlerErr error) {
	handlerName := "embed_batch_text"
	telemetry := newKafkaHandlerTelemetry(a.metricsKafka, a.logger, msg, handlerName, "panic recovered in dlmodel batch text handler")
	telemetry.start()
	defer telemetry.done()
	defer telemetry.recover(&handlerErr)

	var request dtos.EmbedBatchTextRequest
	if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
		telemetry.observe("invalid_payload")
		a.logger.Error("internal.bootstrap.DLModelApp.handleBatchText invalid batch text payload", err, "topic", msg.Topic, "offset", msg.Offset)
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
		telemetry.observe("failed")
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
			telemetry.retry("publish_error")
			a.logger.Error("publish batch text result failed", publishErr, "result_topic", a.topics.BatchTextResult)
		}
	}
	if status == "success" {
		telemetry.observe("success")
	}
	a.logger.Info("batch text pipeline completed", "topic", msg.Topic, "status", status, "count", len(request.Texts), "dimension", dimension, "latency_ms", time.Since(telemetry.startedAt).Milliseconds())
	return nil
}

func (a *DLModelApp) handleBatchImage(ctx context.Context, msg ports.ConsumeMessage) (handlerErr error) {
	handlerName := "embed_batch_image"
	telemetry := newKafkaHandlerTelemetry(a.metricsKafka, a.logger, msg, handlerName, "panic recovered in dlmodel batch image handler")
	telemetry.start()
	defer telemetry.done()
	defer telemetry.recover(&handlerErr)

	var request dtos.EmbedBatchImageRequest
	if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
		telemetry.observe("invalid_payload")
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
		telemetry.observe("failed")
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
			telemetry.retry("publish_error")
			a.logger.Error("publish batch image result failed", publishErr, "result_topic", a.topics.BatchImageResult)
		}
	}
	if status == "success" {
		telemetry.observe("success")
	}
	a.logger.Info("batch image pipeline completed", "topic", msg.Topic, "status", status, "count", len(request.Images), "dimension", dimension, "latency_ms", time.Since(telemetry.startedAt).Milliseconds())
	return nil
}
