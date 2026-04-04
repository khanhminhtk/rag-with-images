package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
		log.Fatalf("failed to load config: %v", err)
	}

	appLogger, err := util.NewFileLogger("logs/minio_service.log", slog.LevelInfo)
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}
	defer appLogger.Close()

	minioClient, err := infraMinio.NewMinioCleant(appLogger, infraMinio.Config{
		Endpoint:  cfg.MinIO.Endpoint,
		AccessKey: cfg.MinIO.AccessKey,
		SecretKey: cfg.MinIO.SecretKey,
		UseSSL:    cfg.MinIO.UseSSL,
		Region:    cfg.MinIO.Region,
	})
	if err != nil {
		appLogger.Error("cmd.minio_service.main: failed to create minio client", err)
		log.Fatalf("failed to create minio client: %v", err)
	}

	minioStorage := infraMinio.NewMinIOStorage(*minioClient, appLogger)

	uploadBucket := cfg.MinIO.Bucket("upload")
	presignBucket := cfg.MinIO.Bucket("presign")
	deleteBucket := cfg.MinIO.Bucket("delete")
	if deleteBucket == "" {
		deleteBucket = uploadBucket
	}
	appLogger.Info(
		"cmd.minio_service.main: resolved bucket config",
		"upload_bucket", uploadBucket,
		"presign_bucket", presignBucket,
		"delete_bucket", deleteBucket,
		"default_bucket", cfg.MinIO.DefaultBucket,
	)

	deleteFileUseCase := (&useCaseMinio.DeleteFileInputUseCase{}).NewDeleteFileInputUseCase(
		deleteBucket,
		appLogger,
		minioStorage,
	)
	presignUseCase := (&useCaseMinio.PresignGetObjectUseCase{}).NewPresignGetObjectUseCase(
		presignBucket,
		time.Duration(cfg.MinIO.PresignExpiryS)*time.Second,
		appLogger,
		minioStorage,
	)
	uploadUseCase := useCaseMinio.NewUploadLocalFileToMinIOUseCase(
		uploadBucket,
		appLogger,
		minioStorage,
	)

	minioService := grpcAdapter.NewMinioService(deleteFileUseCase, presignUseCase, appLogger)

	grpcPort := getenv("MINIO_GRPC_PORT", "50052")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		appLogger.Error("cmd.minio_service.main: failed to listen grpc port", err, "port", grpcPort)
		log.Fatalf("failed to listen grpc port %s: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMinioServiceServer(grpcServer, minioService)
	reflection.Register(grpcServer)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kafkaUploadTopic := getenv("MINIO_KAFKA_UPLOAD_TOPIC", "minio.upload.request")
	kafkaUploadGroup := getenv("MINIO_KAFKA_UPLOAD_GROUP", "service-minio-upload")
	kafkaResultTopic := getenv("MINIO_KAFKA_RESULT_TOPIC", "minio.upload.result")

	producer, consumer, err := kafkaAdapter.NewInfraAdapters(kafkaAdapter.InfraAdapterConfig{
		Brokers:     parseBrokers(getenv("KAFKA_BROKERS", "localhost:9092")),
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
		appLogger.Error("cmd.minio_service.main: failed to initialize kafka adapters", err)
		log.Fatalf("failed to initialize kafka adapters: %v", err)
	}
	defer producer.Close()
	defer consumer.Close()

	consumerErrCh := consumer.Start(rootCtx, kafkaUploadTopic, kafkaUploadGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		var uploadReq dtos.UploadFileMinioRequest
		if err := json.Unmarshal(msg.Message.Value, &uploadReq); err != nil {
			appLogger.Error("cmd.minio_service.main: invalid upload request payload", err, "topic", msg.Topic, "offset", msg.Offset)
			return nil
		}

		_, uploadErr := uploadUseCase.Execute(ctx, &uploadReq)
		status := "success"
		message := "upload completed"
		if uploadErr != nil {
			status = "failed"
			message = uploadErr.Error()
			appLogger.Error(
				"cmd.minio_service.main: upload use case failed",
				uploadErr,
				"topic", msg.Topic,
				"offset", msg.Offset,
				"url_download", uploadReq.UrlDownload,
			)
		}

		if kafkaResultTopic != "" {
			publishErr := producer.PublishJSON(ctx, kafkaResultTopic, msg.Message.Key, map[string]any{
				"status":          status,
				"message":         message,
				"url_download":    uploadReq.UrlDownload,
				"folder_download": uploadReq.FolderDownload,
				"at":              time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic": kafkaUploadTopic,
			})
			if publishErr != nil {
				appLogger.Error("cmd.minio_service.main: publish upload result failed", publishErr, "result_topic", kafkaResultTopic)
			}
		}

		return nil
	})

	go func() {
		appLogger.Info("cmd.minio_service.main: grpc server listening", "port", grpcPort)
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			appLogger.Error("cmd.minio_service.main: grpc serve failed", serveErr)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		appLogger.Info("cmd.minio_service.main: received shutdown signal")
	case err := <-consumerErrCh:
		if err != nil {
			appLogger.Error("cmd.minio_service.main: kafka consumer stopped with error", err)
		}
	}

	cancel()
	grpcServer.GracefulStop()
	appLogger.Info("cmd.minio_service.main: service stopped")
}

func parseBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		broker := strings.TrimSpace(part)
		if broker == "" {
			continue
		}
		brokers = append(brokers, broker)
	}
	if len(brokers) == 0 {
		return []string{"localhost:9092"}
	}
	return brokers
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
