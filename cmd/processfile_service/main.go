package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	trainingfile "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/training_file"
	"rag_imagetotext_texttoimage/internal/bootstrap"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
)

type processFileKafkaRequest struct {
	dtos.ProcessAndIngestRequest
	CorrelationID string `json:"correlation_id,omitempty"`
}

func main() {
	cfg, appLogger, err := bootstrap.BuildConfigAndLogger(bootstrap.CmdRuntimeOptions{
		Namespace: "processfile_service",
		EnvPath:   "./config/.env",
		YamlPath:  "./config/config.yaml",
		LogLevel:  slog.LevelInfo,
		ResolveLogPath: func(_ *util.Config) string {
			return "logs/processfile_service.log"
		},
	})
	if err != nil {
		util.Fatalf("failed to bootstrap processfile runtime: %v", err)
	}
	defer appLogger.Close()
	logPath := "logs/processfile_service.log"
	appLogger.Info("process file service bootstrap started", "log_path", logPath)
	appLogger.Info(
		"process file runtime config",
		"batch_size", cfg.FileTraining.BatchSize,
		"marker_dev_mode", cfg.FileTraining.MarkerDevMode,
	)

	topics := cfg.FileTraining.Topics
	topics.ProcessFileRequest = strings.TrimSpace(topics.ProcessFileRequest)
	topics.ProcessFileGroup = strings.TrimSpace(topics.ProcessFileGroup)
	topics.ProcessFileResult = strings.TrimSpace(topics.ProcessFileResult)

	if topics.ProcessFileRequest == "" {
		util.Fatalf("process_file_request topic is empty")
	}
	if topics.ProcessFileGroup == "" {
		util.Fatalf("process_file_group is empty")
	}
	appLogger.Info(
		"process file kafka topic config",
		"request_topic", topics.ProcessFileRequest,
		"group_id", topics.ProcessFileGroup,
		"result_topic", topics.ProcessFileResult,
	)

	kafkaClient, err := infraKafka.NewKafkaClient(infraKafka.KafkaConfig{
		Brokers:     cfg.Kafka.Brokers,
		DialTimeout: 10 * time.Second,
	}, appLogger)
	if err != nil {
		util.Fatalf("failed to create kafka client: %v", err)
	}

	publisher, err := infraKafka.NewPublisher(kafkaClient, infraKafka.PublisherConfig{
		RequiredAcks: -1,
	}, appLogger)
	if err != nil {
		util.Fatalf("failed to create kafka publisher: %v", err)
	}
	defer publisher.Close()

	consumer, err := infraKafka.NewKafkaConsumer(kafkaClient, infraKafka.ConsumerConfig{
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    -2,
	}, appLogger)
	if err != nil {
		util.Fatalf("failed to create kafka consumer: %v", err)
	}
	defer consumer.Close()
	appLogger.Info("process file kafka adapters ready")

	producerAdapter := kafkaAdapter.NewProducerAdapter(publisher)
	consumerAdapter := kafkaAdapter.NewConsumerAdapter(consumer, appLogger)
	trainingFileUseCase := trainingfile.NewTrainingFileUseCase(appLogger, publisher, consumer, *cfg)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerErrCh := consumerAdapter.Start(rootCtx, topics.ProcessFileRequest, topics.ProcessFileGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		startedAt := time.Now()
		correlationID := resolveCorrelationID("", msg)

		var req processFileKafkaRequest
		if err := decodeProcessFileKafkaRequest(msg.Message.Value, &req); err != nil {
			appLogger.Error("invalid process file request payload", err, "topic", msg.Topic, "offset", msg.Offset, "correlation_id", correlationID)
			if topics.ProcessFileResult != "" {
				publishErr := producerAdapter.PublishJSON(ctx, topics.ProcessFileResult, []byte(correlationID), map[string]any{
					"success":         false,
					"message":         "invalid request payload: " + err.Error(),
					"correlation_id":  correlationID,
					"uuid":            "",
					"collection_name": "",
					"at":              time.Now().UTC().Format(time.RFC3339),
				}, map[string]string{
					"source_topic":   topics.ProcessFileRequest,
					"correlation_id": correlationID,
				})
				if publishErr != nil {
					appLogger.Error("publish invalid process file request result failed", publishErr, "result_topic", topics.ProcessFileResult, "correlation_id", correlationID)
				}
			}
			return nil
		}

		correlationID = resolveCorrelationID(req.CorrelationID, msg)

		processResult, processErr := trainingFileUseCase.ProcessAndIngest(ctx, &req.ProcessAndIngestRequest)
		success := processErr == nil && processResult.Success
		message := "process and ingest completed"
		if processErr != nil {
			message = processErr.Error()
			appLogger.Error("process and ingest failed", processErr, "uuid", req.UUID, "correlation_id", correlationID)
		}

		if topics.ProcessFileResult != "" {
			publishErr := producerAdapter.PublishJSON(ctx, topics.ProcessFileResult, []byte(correlationID), map[string]any{
				"success":         success,
				"message":         message,
				"correlation_id":  correlationID,
				"uuid":            req.UUID,
				"collection_name": req.UUID,
				"result":          processResult,
				"at":              time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic":   topics.ProcessFileRequest,
				"correlation_id": correlationID,
			})
			if publishErr != nil {
				appLogger.Error("publish process file result failed", publishErr, "result_topic", topics.ProcessFileResult, "correlation_id", correlationID)
			}
		}

		appLogger.Info(
			"process file pipeline completed",
			"topic", msg.Topic,
			"uuid", req.UUID,
			"correlation_id", correlationID,
			"success", success,
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)
		return nil
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		appLogger.Info("received shutdown signal")
	case err := <-consumerErrCh:
		if err != nil {
			appLogger.Error("process file consumer stopped with error", err)
		}
	}

	cancel()
	waitConsumerStopped(consumerErrCh, appLogger, 15*time.Second)
	appLogger.Info("process file service stopped")
}

func waitConsumerStopped(errCh <-chan error, logger util.Logger, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err, ok := <-errCh:
		if !ok {
			return
		}
		if err != nil {
			logger.Error("consumer stop returned error", err)
		}
	case <-timer.C:
		logger.Info("consumer stop wait timeout", "timeout_ms", timeout.Milliseconds())
	}
}

func decodeProcessFileKafkaRequest(raw []byte, out *processFileKafkaRequest) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid trailing payload")
	}
	return nil
}

func resolveCorrelationID(requestCorrelationID string, msg ports.ConsumeMessage) string {
	correlationID := strings.TrimSpace(requestCorrelationID)
	if correlationID == "" {
		correlationID = strings.TrimSpace(msg.Message.Headers["correlation_id"])
	}
	if correlationID == "" {
		correlationID = strings.TrimSpace(string(msg.Message.Key))
	}
	if correlationID == "" {
		correlationID = fmt.Sprintf("process-file-%d", time.Now().UnixNano())
	}
	return correlationID
}
