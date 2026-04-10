package main

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

func main() {
	configLoader := util.NewConfigLoader("./config/.env", "./config/config.yaml")
	cfg, err := configLoader.Load()
	if err != nil {
		util.Fatalf("failed to load config: %v", err)
	}

	logPath := filepath.Join("cmd", "minio_service", "logs", "minio_service.log")
	appLogger, err := util.NewFileLogger(logPath, slog.LevelInfo)
	if err != nil {
		util.Fatalf("failed to create logger: %v", err)
	}
	defer appLogger.Close()
	appLogger.Info("minio service bootstrap started", "grpc_port", cfg.MinIOService.GRPCPort, "endpoint", cfg.MinIOService.Endpoint, "log_path", logPath)

	minioClient, err := infraMinio.NewMinioCleant(appLogger, infraMinio.Config{
		Endpoint:  cfg.MinIOService.Endpoint,
		AccessKey: cfg.MinIOService.AccessKey,
		SecretKey: cfg.MinIOService.SecretKey,
		UseSSL:    cfg.MinIOService.UseSSL,
		Region:    cfg.MinIOService.Region,
	})
	if err != nil {
		appLogger.Error("failed to create minio client", err)
		util.Fatalf("failed to create minio client: %v", err)
	}
	appLogger.Info("minio client ready", "endpoint", cfg.MinIOService.Endpoint, "region", cfg.MinIOService.Region)

	minioStorage := infraMinio.NewMinIOStorage(*minioClient, appLogger)

	runtimeBucket := cfg.MinIOService.Bucket("default")
	if runtimeBucket == "" {
		runtimeBucket = cfg.MinIOService.DefaultBucket
	}
	if runtimeBucket == "" {
		util.Fatalf("minio runtime bucket is empty")
	}

	appLogger.Info(
		"resolved bucket config",
		"runtime_bucket", runtimeBucket,
		"default_bucket", cfg.MinIOService.DefaultBucket,
	)

	if err := minioStorage.EnsureBucket(context.Background(), runtimeBucket); err != nil {
		appLogger.Error("failed to ensure bucket", err, "bucket", runtimeBucket)
		util.Fatalf("failed to ensure bucket %s: %v", runtimeBucket, err)
	}

	deleteFileUseCase := (&useCaseMinio.DeleteFileInputUseCase{}).NewDeleteFileInputUseCase(
		runtimeBucket,
		appLogger,
		minioStorage,
	)
	presignUseCase := (&useCaseMinio.PresignGetObjectUseCase{}).NewPresignGetObjectUseCase(
		runtimeBucket,
		time.Duration(cfg.MinIOService.PresignExpiryS)*time.Second,
		appLogger,
		minioStorage,
	)
	uploadUseCase := useCaseMinio.NewUploadLocalFileToMinIOUseCase(
		runtimeBucket,
		appLogger,
		minioStorage,
	)

	minioService := grpcAdapter.NewMinioService(deleteFileUseCase, presignUseCase, appLogger)

	grpcPort := cfg.MinIOService.GRPCPort
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		appLogger.Error("failed to listen grpc port", err, "port", grpcPort)
		util.Fatalf("failed to listen grpc port %s: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMinioServiceServer(grpcServer, minioService)
	reflection.Register(grpcServer)
	appLogger.Info("minio grpc reflection enabled")

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	topics := cfg.MinIOService.Topics
	topics.UploadRequest = strings.TrimSpace(topics.UploadRequest)
	topics.UploadGroup = strings.TrimSpace(topics.UploadGroup)
	topics.UploadResult = strings.TrimSpace(topics.UploadResult)
	if topics.UploadRequest == "" {
		util.Fatalf("minio upload request topic is empty")
	}
	if topics.UploadGroup == "" {
		util.Fatalf("minio upload group is empty")
	}
	appLogger.Info(
		"minio kafka topic config",
		"upload_request", topics.UploadRequest,
		"upload_group", topics.UploadGroup,
		"upload_result", topics.UploadResult,
	)

	producer, consumer, err := kafkaAdapter.NewInfraAdapters(kafkaAdapter.InfraAdapterConfig{
		Brokers:     cfg.Kafka.Brokers,
		DialTimeout: 10 * time.Second,
		Publisher: infraKafka.PublisherConfig{
			RequiredAcks: -1,
		},
		Consumer: infraKafka.ConsumerConfig{
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
			StartOffset:    -2,
		},
	}, appLogger)
	if err != nil {
		appLogger.Error("failed to initialize kafka adapters", err)
		util.Fatalf("failed to initialize kafka adapters: %v", err)
	}
	defer producer.Close()
	defer consumer.Close()

	consumerErrCh := consumer.Start(rootCtx, topics.UploadRequest, topics.UploadGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		startedAt := time.Now()
		var uploadReq dtos.UploadFileMinioRequest
		if err := json.Unmarshal(msg.Message.Value, &uploadReq); err != nil {
			appLogger.Error("invalid upload request payload", err, "topic", msg.Topic, "offset", msg.Offset)
			return nil
		}

		_, uploadErr := uploadUseCase.Execute(ctx, &uploadReq)
		status := "success"
		message := "upload completed"
		if uploadErr != nil {
			status = "failed"
			message = uploadErr.Error()
			appLogger.Error(
				"upload use case failed",
				uploadErr,
				"topic", msg.Topic,
				"offset", msg.Offset,
				"url_download", uploadReq.UrlDownload,
			)
		}

		if topics.UploadResult != "" {
			publishErr := producer.PublishJSON(ctx, topics.UploadResult, msg.Message.Key, map[string]any{
				"status":          status,
				"message":         message,
				"url_download":    uploadReq.UrlDownload,
				"folder_download": uploadReq.FolderDownload,
				"at":              time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic": topics.UploadRequest,
			})
			if publishErr != nil {
				appLogger.Error("publish upload result failed", publishErr, "result_topic", topics.UploadResult)
			}
		}
		appLogger.Info(
			"minio upload pipeline completed",
			"topic", msg.Topic,
			"status", status,
			"url_download", uploadReq.UrlDownload,
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)

		return nil
	})

	go func() {
		appLogger.Info("minio grpc server listening", "port", grpcPort)
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			appLogger.Error("grpc serve failed", serveErr)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		appLogger.Info("received shutdown signal")
	case err := <-consumerErrCh:
		if err != nil {
			appLogger.Error("kafka consumer stopped with error", err)
		}
	}

	cancel()
	grpcServer.GracefulStop()
	appLogger.Info("service stopped")
}
