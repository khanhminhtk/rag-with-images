package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	"rag_imagetotext_texttoimage/internal/infra/monitoring"
	"rag_imagetotext_texttoimage/internal/util"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	metricsKafka := monitoring.NewMetrics()
	metricsSrv := newProcessFileMetricsHTTPServer(cfg)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerErrCh := consumerAdapter.Start(rootCtx, topics.ProcessFileRequest, topics.ProcessFileGroup, func(ctx context.Context, msg ports.ConsumeMessage) (handlerErr error) {
		handlerName := "process_file_ingest"
		telemetry := newProcessFileTelemetry(metricsKafka, appLogger, msg, handlerName, "panic recovered in process file handler")
		telemetry.start()
		defer telemetry.done()
		defer telemetry.recover(&handlerErr)

		correlationID := resolveCorrelationID("", msg)

		var req processFileKafkaRequest
		if err := decodeProcessFileKafkaRequest(msg.Message.Value, &req); err != nil {
			telemetry.observe("invalid_payload")
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
					telemetry.retry("publish_error")
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
			telemetry.observe("failed")
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
				telemetry.retry("publish_error")
				appLogger.Error("publish process file result failed", publishErr, "result_topic", topics.ProcessFileResult, "correlation_id", correlationID)
			}
		}
		if success {
			telemetry.observe("success")
		}

		appLogger.Info(
			"process file pipeline completed",
			"topic", msg.Topic,
			"uuid", req.UUID,
			"correlation_id", correlationID,
			"success", success,
			"latency_ms", time.Since(telemetry.startedAt).Milliseconds(),
		)
		return nil
	})

	metricsErrCh := make(chan error, 1)
	go func() {
		if metricsSrv == nil {
			return
		}
		appLogger.Info("process file metrics server listening", "addr", metricsSrv.Addr)
		if serveErr := metricsSrv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			metricsErrCh <- serveErr
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		appLogger.Info("received shutdown signal")
	case err := <-metricsErrCh:
		if err != nil {
			appLogger.Error("process file metrics server stopped with error", err)
		}
	case err := <-consumerErrCh:
		if err != nil {
			appLogger.Error("process file consumer stopped with error", err)
		}
	}

	cancel()
	waitConsumerStopped(consumerErrCh, appLogger, 15*time.Second)
	if metricsSrv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = metricsSrv.Shutdown(shutdownCtx)
		shutdownCancel()
	}
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

func newProcessFileMetricsHTTPServer(cfg *util.Config) *http.Server {
	if cfg == nil {
		return nil
	}
	host := strings.TrimSpace(cfg.FileTraining.IDMonitoring)
	if host == "" {
		host = "0.0.0.0"
	}
	port := strings.TrimSpace(cfg.FileTraining.PortMetricGRPC)
	if port == "" {
		port = "9105"
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: mux,
	}
}

type processFileTelemetry struct {
	metrics         *monitoring.Metrics
	logger          util.Logger
	msg             ports.ConsumeMessage
	handlerName     string
	panicLogMessage string
	startedAt       time.Time
}

func newProcessFileTelemetry(metrics *monitoring.Metrics, logger util.Logger, msg ports.ConsumeMessage, handlerName string, panicLogMessage string) *processFileTelemetry {
	return &processFileTelemetry{
		metrics:         metrics,
		logger:          logger,
		msg:             msg,
		handlerName:     handlerName,
		panicLogMessage: panicLogMessage,
		startedAt:       time.Now(),
	}
}

func (t *processFileTelemetry) start() {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.InFlight.Inc()
	t.metrics.QueueLength.Set(float64(t.msg.Lag))
}

func (t *processFileTelemetry) done() {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.InFlight.Dec()
}

func (t *processFileTelemetry) observe(status string) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.ProcessedTotal.WithLabelValues(t.msg.Topic, t.handlerName, status).Inc()
	t.metrics.ProcessingTime.WithLabelValues(t.msg.Topic, t.handlerName, status).Observe(time.Since(t.startedAt).Seconds())
}

func (t *processFileTelemetry) retry(errorType string) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RetryTotal.WithLabelValues(t.msg.Topic, t.handlerName, errorType).Inc()
}

func (t *processFileTelemetry) recover(handlerErr *error) {
	if t == nil {
		return
	}
	recovered := recover()
	if recovered == nil {
		return
	}

	panicErr := fmt.Errorf("panic recovered: %v", recovered)
	if t.metrics != nil {
		t.metrics.PanicsTotal.WithLabelValues(t.msg.Topic, t.handlerName).Inc()
		t.observe("panic")
	}
	if handlerErr != nil {
		*handlerErr = panicErr
	}
	if t.logger != nil {
		t.logger.Error(t.panicLogMessage, panicErr, "topic", t.msg.Topic, "offset", t.msg.Offset)
	}
}
