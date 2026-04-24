package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	useCaseMinio "rag_imagetotext_texttoimage/internal/application/use_cases/minio"
	"rag_imagetotext_texttoimage/internal/infra/monitoring"
	"rag_imagetotext_texttoimage/internal/util"
)

type MinioApp struct {
	projectRoot string
	cfg         *util.Config
	logger      util.Logger
	grpcPort    string
	grpcServer  *grpc.Server
	listener    net.Listener
	producer    *kafkaAdapter.ProducerAdapter
	consumer    *kafkaAdapter.ConsumerAdapter
	uploadUC    *useCaseMinio.UploadLocalFileToMinIOUseCase
	topics      util.MinIOTopics
	metrics     *monitoring.Metrics
	metricsSrv  *http.Server
}

type minioUseCases struct {
	deleteUC  *useCaseMinio.DeleteFileInputUseCase
	presignUC *useCaseMinio.PresignGetObjectUseCase
	uploadUC  *useCaseMinio.UploadLocalFileToMinIOUseCase
}

type minioKafkaInfra struct {
	producer *kafkaAdapter.ProducerAdapter
	consumer *kafkaAdapter.ConsumerAdapter
}

func NewMinioApp() (*MinioApp, error) {
	projectRoot := ResolveProjectRoot()
	container := NewContainer()
	registry := NewRegistry(container, "minio_service")

	configKey := registry.Key("config")
	loggerKey := registry.Key("logger")
	runtimeBucketKey := registry.Key("runtime.bucket")
	storageKey := registry.Key("storage")
	useCasesKey := registry.Key("use_cases")
	topicsKey := registry.Key("topics")
	kafkaInfraKey := registry.Key("kafka.infra")
	grpcServiceKey := registry.Key("grpc.service")
	grpcServerKey := registry.Key("grpc.server")
	listenerKey := registry.Key("grpc.listener")
	metricsHTTPKey := registry.Key("metrics.http_server")

	if err := registerMinioBindings(container, minioBindingKeys{
		ConfigKey:        configKey,
		LoggerKey:        loggerKey,
		RuntimeBucketKey: runtimeBucketKey,
		StorageKey:       storageKey,
		UseCasesKey:      useCasesKey,
		TopicsKey:        topicsKey,
		KafkaInfraKey:    kafkaInfraKey,
		GRPCServiceKey:   grpcServiceKey,
		GRPCServerKey:    grpcServerKey,
		ListenerKey:      listenerKey,
		MetricsHTTPKey:   metricsHTTPKey,
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
	topics, err := ResolveAs[util.MinIOTopics](container, topicsKey)
	if err != nil {
		logger.Close()
		return nil, err
	}
	kafkaInfra, err := ResolveAs[minioKafkaInfra](container, kafkaInfraKey)
	if err != nil {
		logger.Close()
		return nil, err
	}
	useCases, err := ResolveAs[minioUseCases](container, useCasesKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		logger.Close()
		return nil, err
	}
	grpcServer, err := ResolveAs[*grpc.Server](container, grpcServerKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		logger.Close()
		return nil, err
	}
	listener, err := ResolveAs[net.Listener](container, listenerKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		logger.Close()
		return nil, err
	}
	metricsSrv, err := ResolveAs[*http.Server](container, metricsHTTPKey)
	if err != nil {
		_ = kafkaInfra.producer.Close()
		_ = kafkaInfra.consumer.Close()
		logger.Close()
		return nil, err
	}

	logger.Info("minio service bootstrap started", "grpc_port", cfg.MinIOService.GRPCPort, "endpoint", cfg.MinIOService.Endpoint, "log_path", ProjectPath("logs", "minio_service.log"))
	logger.Info("minio kafka topic config", "upload_request", topics.UploadRequest, "upload_group", topics.UploadGroup, "upload_result", topics.UploadResult)

	return &MinioApp{
		projectRoot: projectRoot,
		cfg:         cfg,
		logger:      logger,
		grpcPort:    cfg.MinIOService.GRPCPort,
		grpcServer:  grpcServer,
		listener:    listener,
		producer:    kafkaInfra.producer,
		consumer:    kafkaInfra.consumer,
		uploadUC:    useCases.uploadUC,
		topics:      topics,
		metrics:     monitoring.NewMetrics(),
		metricsSrv:  metricsSrv,
	}, nil
}

func (a *MinioApp) Run() error {
	if a == nil {
		return fmt.Errorf("internal.bootstrap.MinioApp.Run app is nil")
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerErrCh := a.consumer.Start(rootCtx, a.topics.UploadRequest, a.topics.UploadGroup, func(ctx context.Context, msg ports.ConsumeMessage) (handlerErr error) {
		handlerName := "minio_upload"
		telemetry := newKafkaHandlerTelemetry(a.metrics, a.logger, msg, handlerName, "panic recovered in minio upload handler")
		telemetry.start()
		defer telemetry.done()
		defer telemetry.recover(&handlerErr)

		var uploadReq dtos.UploadFileMinioRequest
		if err := json.Unmarshal(msg.Message.Value, &uploadReq); err != nil {
			telemetry.observe("invalid_payload")
			a.logger.Error("internal.bootstrap.MinioApp.Run invalid upload request payload", err, "topic", msg.Topic, "offset", msg.Offset)
			return nil
		}

		if folderDownload := strings.TrimSpace(uploadReq.FolderDownload); folderDownload != "" && !filepath.IsAbs(folderDownload) {
			uploadReq.FolderDownload = filepath.Clean(filepath.Join(a.projectRoot, folderDownload))
		}

		_, uploadErr := a.uploadUC.Execute(ctx, &uploadReq)
		status := "success"
		message := "upload completed"
		if uploadErr != nil {
			status = "failed"
			message = uploadErr.Error()
			telemetry.observe("failed")
			a.logger.Error("internal.bootstrap.MinioApp.Run upload use case failed", uploadErr, "topic", msg.Topic, "offset", msg.Offset, "url_download", uploadReq.UrlDownload)
		}

		if a.topics.UploadResult != "" {
			publishErr := a.producer.PublishJSON(ctx, a.topics.UploadResult, msg.Message.Key, map[string]any{
				"status":          status,
				"message":         message,
				"url_download":    uploadReq.UrlDownload,
				"folder_download": uploadReq.FolderDownload,
				"at":              time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{"source_topic": a.topics.UploadRequest})
			if publishErr != nil {
				telemetry.retry("publish_error")
				a.logger.Error("internal.bootstrap.MinioApp.Run publish upload result failed", publishErr, "result_topic", a.topics.UploadResult)
			}
		}
		if status == "success" {
			telemetry.observe("success")
		}
		a.logger.Info("internal.bootstrap.MinioApp.Run minio upload pipeline completed", "topic", msg.Topic, "status", status, "url_download", uploadReq.UrlDownload, "latency_ms", time.Since(telemetry.startedAt).Milliseconds())
		return nil
	})

	go func() {
		a.logger.Info("internal.bootstrap.MinioApp.Run minio grpc server listening", "port", a.grpcPort)
		if serveErr := a.grpcServer.Serve(a.listener); serveErr != nil {
			a.logger.Error("internal.bootstrap.MinioApp.Run grpc serve failed", serveErr)
		}
	}()
	metricsErrCh := make(chan error, 1)
	go func() {
		if a.metricsSrv == nil {
			return
		}
		a.logger.Info("internal.bootstrap.MinioApp.Run minio metrics server listening", "addr", a.metricsSrv.Addr)
		if serveErr := a.metricsSrv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			metricsErrCh <- serveErr
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signalCh:
		a.logger.Info("internal.bootstrap.MinioApp.Run received shutdown signal")
	case err := <-metricsErrCh:
		if err != nil {
			a.logger.Error("internal.bootstrap.MinioApp.Run metrics server stopped with error", err)
		}
	case err := <-consumerErrCh:
		if err != nil {
			a.logger.Error("internal.bootstrap.MinioApp.Run kafka consumer stopped with error", err)
		}
	}

	signal.Stop(signalCh)
	close(signalCh)
	cancel()
	if a.metricsSrv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = a.metricsSrv.Shutdown(shutdownCtx)
		shutdownCancel()
	}
	a.grpcServer.GracefulStop()
	a.logger.Info("internal.bootstrap.MinioApp.Run service stopped")
	return nil
}

func (a *MinioApp) Close() {
	if a == nil {
		return
	}
	if a.producer != nil {
		_ = a.producer.Close()
	}
	if a.consumer != nil {
		_ = a.consumer.Close()
	}
	if a.logger != nil {
		a.logger.Close()
	}
}
