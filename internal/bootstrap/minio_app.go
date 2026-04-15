package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	useCaseMinio "rag_imagetotext_texttoimage/internal/application/use_cases/minio"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	infraMinio "rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
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

	if err := registry.RegisterSingleton(configKey, func(_ Resolver) (any, error) {
		loader := util.NewConfigLoader(ProjectPath("config", ".env"), ProjectPath("config", "config.yaml"))
		return loader.Load()
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(loggerKey, func(_ Resolver) (any, error) {
		return util.NewFileLogger(ProjectPath("logs", "minio_service.log"), slog.LevelInfo)
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(runtimeBucketKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		runtimeBucket := cfg.MinIOService.Bucket("default")
		if runtimeBucket == "" {
			runtimeBucket = cfg.MinIOService.DefaultBucket
		}
		if runtimeBucket == "" {
			return nil, fmt.Errorf("minio runtime bucket is empty")
		}
		return runtimeBucket, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(storageKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		minioClient, err := infraMinio.NewMinioCleant(logger, infraMinio.Config{
			Endpoint:  cfg.MinIOService.Endpoint,
			AccessKey: cfg.MinIOService.AccessKey,
			SecretKey: cfg.MinIOService.SecretKey,
			UseSSL:    cfg.MinIOService.UseSSL,
			Region:    cfg.MinIOService.Region,
		})
		if err != nil {
			logger.Error("failed to create minio client", err)
			return nil, err
		}
		storage := infraMinio.NewMinIOStorage(*minioClient, logger)
		runtimeBucket, err := ResolveAs[string](r, runtimeBucketKey)
		if err != nil {
			return nil, err
		}
		if err := storage.EnsureBucket(context.Background(), runtimeBucket); err != nil {
			logger.Error("failed to ensure bucket", err, "bucket", runtimeBucket)
			return nil, err
		}
		return storage, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(useCasesKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		runtimeBucket, err := ResolveAs[string](r, runtimeBucketKey)
		if err != nil {
			return nil, err
		}
		storage, err := ResolveAs[*infraMinio.MinIOStorage](r, storageKey)
		if err != nil {
			return nil, err
		}
		deleteUC := (&useCaseMinio.DeleteFileInputUseCase{}).NewDeleteFileInputUseCase(runtimeBucket, logger, storage)
		presignUC := (&useCaseMinio.PresignGetObjectUseCase{}).NewPresignGetObjectUseCase(runtimeBucket, time.Duration(cfg.MinIOService.PresignExpiryS)*time.Second, logger, storage)
		uploadUC := useCaseMinio.NewUploadLocalFileToMinIOUseCase(runtimeBucket, logger, storage)
		return minioUseCases{deleteUC: deleteUC, presignUC: presignUC, uploadUC: uploadUC}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(topicsKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		topics := cfg.MinIOService.Topics
		topics.UploadRequest = strings.TrimSpace(topics.UploadRequest)
		topics.UploadGroup = strings.TrimSpace(topics.UploadGroup)
		topics.UploadResult = strings.TrimSpace(topics.UploadResult)
		if topics.UploadRequest == "" {
			return nil, fmt.Errorf("minio upload request topic is empty")
		}
		if topics.UploadGroup == "" {
			return nil, fmt.Errorf("minio upload group is empty")
		}
		return topics, nil
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
		return minioKafkaInfra{producer: producer, consumer: consumer}, nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(grpcServiceKey, func(r Resolver) (any, error) {
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		ucs, err := ResolveAs[minioUseCases](r, useCasesKey)
		if err != nil {
			return nil, err
		}
		return grpcAdapter.NewMinioService(ucs.deleteUC, ucs.presignUC, logger), nil
	}); err != nil {
		return nil, err
	}

	if err := registry.RegisterSingleton(grpcServerKey, func(r Resolver) (any, error) {
		service, err := ResolveAs[*grpcAdapter.MinioService](r, grpcServiceKey)
		if err != nil {
			return nil, err
		}
		logger, err := ResolveAs[util.Logger](r, loggerKey)
		if err != nil {
			return nil, err
		}
		grpcServer := grpc.NewServer()
		pb.RegisterMinioServiceServer(grpcServer, service)
		reflection.Register(grpcServer)
		logger.Info("minio grpc reflection enabled")
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
		lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.MinIOService.GRPCPort))
		if err != nil {
			logger.Error("failed to listen grpc port", err, "port", cfg.MinIOService.GRPCPort)
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
	}, nil
}

func (a *MinioApp) Run() error {
	if a == nil {
		return fmt.Errorf("internal.bootstrap.MinioApp.Run app is nil")
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerErrCh := a.consumer.Start(rootCtx, a.topics.UploadRequest, a.topics.UploadGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		startedAt := time.Now()
		var uploadReq dtos.UploadFileMinioRequest
		if err := json.Unmarshal(msg.Message.Value, &uploadReq); err != nil {
			a.logger.Error("invalid upload request payload", err, "topic", msg.Topic, "offset", msg.Offset)
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
			a.logger.Error("upload use case failed", uploadErr, "topic", msg.Topic, "offset", msg.Offset, "url_download", uploadReq.UrlDownload)
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
				a.logger.Error("publish upload result failed", publishErr, "result_topic", a.topics.UploadResult)
			}
		}
		a.logger.Info("minio upload pipeline completed", "topic", msg.Topic, "status", status, "url_download", uploadReq.UrlDownload, "latency_ms", time.Since(startedAt).Milliseconds())
		return nil
	})

	go func() {
		a.logger.Info("minio grpc server listening", "port", a.grpcPort)
		if serveErr := a.grpcServer.Serve(a.listener); serveErr != nil {
			a.logger.Error("grpc serve failed", serveErr)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signalCh:
		a.logger.Info("received shutdown signal")
	case err := <-consumerErrCh:
		if err != nil {
			a.logger.Error("kafka consumer stopped with error", err)
		}
	}

	signal.Stop(signalCh)
	close(signalCh)
	cancel()
	a.grpcServer.GracefulStop()
	a.logger.Info("service stopped")
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
