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
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/cgo"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

func main() {
	configLoader := util.NewConfigLoader("./config/.env", "./config/config.yaml")
	if _, err := configLoader.Load(); err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	pathLog := getenv("DLMODEL_LOG_PATH", "./logs/dlmodel_service.log")

	appLogger, err := util.NewFileLogger(pathLog, slog.LevelInfo)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Close()

	jinaConfigPath := getenv("DLMODEL_JINA_CONFIG", "./config/jina_config.yaml")
	jinaAdapter, err := cgo.NewJinaAdapter(jinaConfigPath, appLogger)
	if err != nil {
		appLogger.Error("cmd.dlmodel_service.main: failed to initialize Jina adapter", err, "config_path", jinaConfigPath)
		log.Fatalf("failed to initialize Jina adapter: %v", err)
	}
	defer jinaAdapter.Close()

	dlModelService := grpcAdapter.NewDLModelService(appLogger, jinaAdapter)
	grpcPort := getenv("DLMODEL_GRPC_PORT", "50053")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		appLogger.Error("cmd.dlmodel_service.main: failed to listen grpc port", err, "port", grpcPort)
		log.Fatalf("failed to listen grpc port %s: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterDeepLearningServiceServer(grpcServer, dlModelService)
	reflection.Register(grpcServer)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		appLogger.Error("cmd.dlmodel_service.main: failed to initialize kafka adapters", err)
		log.Fatalf("failed to initialize kafka adapters: %v", err)
	}
	defer producer.Close()
	defer consumer.Close()

	batchTextTopic := getenv("DLMODEL_KAFKA_BATCH_TEXT_TOPIC", "dlmodel.embed.batch_text.request")
	batchTextGroup := getenv("DLMODEL_KAFKA_BATCH_TEXT_GROUP", "service-dlmodel-batch-text")
	batchTextResultTopic := getenv("DLMODEL_KAFKA_BATCH_TEXT_RESULT_TOPIC", "dlmodel.embed.batch_text.result")

	batchImageTopic := getenv("DLMODEL_KAFKA_BATCH_IMAGE_TOPIC", "dlmodel.embed.batch_image.request")
	batchImageGroup := getenv("DLMODEL_KAFKA_BATCH_IMAGE_GROUP", "service-dlmodel-batch-image")
	batchImageResultTopic := getenv("DLMODEL_KAFKA_BATCH_IMAGE_RESULT_TOPIC", "dlmodel.embed.batch_image.result")

	batchTextErrCh := consumer.Start(rootCtx, batchTextTopic, batchTextGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		var request struct {
			Texts []string `json:"texts"`
		}
		if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
			appLogger.Error("cmd.dlmodel_service.main: invalid batch text payload", err, "topic", msg.Topic, "offset", msg.Offset)
			return nil
		}

		embeddings, runErr := jinaAdapter.EmbedBatchText(request.Texts)
		status := "success"
		message := "batch text embedding completed"
		dimension := 0
		if len(embeddings) > 0 {
			dimension = len(embeddings[0])
		}
		if runErr != nil {
			status = "failed"
			message = runErr.Error()
			appLogger.Error("cmd.dlmodel_service.main: batch text embedding failed", runErr, "topic", msg.Topic, "offset", msg.Offset)
		}

		if batchTextResultTopic != "" {
			publishErr := producer.PublishJSON(ctx, batchTextResultTopic, msg.Message.Key, map[string]any{
				"status":     status,
				"message":    message,
				"count":      len(request.Texts),
				"dimension":  dimension,
				"embeddings": embeddings,
				"at":         time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic": batchTextTopic,
			})
			if publishErr != nil {
				appLogger.Error("cmd.dlmodel_service.main: publish batch text result failed", publishErr, "result_topic", batchTextResultTopic)
			}
		}

		return nil
	})

	batchImageErrCh := consumer.Start(rootCtx, batchImageTopic, batchImageGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		var request struct {
			Images   [][]byte `json:"images"`
			Width    int      `json:"width"`
			Height   int      `json:"height"`
			Channels int      `json:"channels"`
		}
		if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
			appLogger.Error("cmd.dlmodel_service.main: invalid batch image payload", err, "topic", msg.Topic, "offset", msg.Offset)
			return nil
		}

		embeddings, runErr := jinaAdapter.EmbedBatchImage(request.Images, request.Width, request.Height, request.Channels)
		status := "success"
		message := "batch image embedding completed"
		dimension := 0
		if len(embeddings) > 0 {
			dimension = len(embeddings[0])
		}
		if runErr != nil {
			status = "failed"
			message = runErr.Error()
			appLogger.Error("cmd.dlmodel_service.main: batch image embedding failed", runErr, "topic", msg.Topic, "offset", msg.Offset)
		}

		if batchImageResultTopic != "" {
			publishErr := producer.PublishJSON(ctx, batchImageResultTopic, msg.Message.Key, map[string]any{
				"status":     status,
				"message":    message,
				"count":      len(request.Images),
				"dimension":  dimension,
				"embeddings": embeddings,
				"at":         time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic": batchImageTopic,
			})
			if publishErr != nil {
				appLogger.Error("cmd.dlmodel_service.main: publish batch image result failed", publishErr, "result_topic", batchImageResultTopic)
			}
		}

		return nil
	})

	grpcErrCh := make(chan error, 1)
	go func() {
		appLogger.Info("cmd.dlmodel_service.main: grpc server listening", "port", grpcPort)
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			grpcErrCh <- serveErr
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		appLogger.Info("cmd.dlmodel_service.main: received shutdown signal")
	case err := <-grpcErrCh:
		if err != nil {
			appLogger.Error("cmd.dlmodel_service.main: grpc server stopped with error", err)
		}
	case err := <-batchTextErrCh:
		if err != nil {
			appLogger.Error("cmd.dlmodel_service.main: batch text consumer stopped with error", err)
		}
	case err := <-batchImageErrCh:
		if err != nil {
			appLogger.Error("cmd.dlmodel_service.main: batch image consumer stopped with error", err)
		}
	}

	cancel()
	grpcServer.GracefulStop()
	appLogger.Info("cmd.dlmodel_service.main: service stopped")
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
