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
	"gopkg.in/yaml.v2"

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
	cfg, err := configLoader.Load()
	if err != nil {
		util.Fatalf("failed to load configuration: %v", err)
	}

	appLogger, err := util.NewFileLogger(cfg.EmbeddingService.LogPath, slog.LevelInfo)
	if err != nil {
		util.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Close()

	appLogger.Info("Initializing Jina CLIP adapter", "config_path", cfg.EmbeddingService.JinaConfigPath)
	runtimeConfigPath, cleanupRuntimeConfig, err := prepareJinaRuntime(cfg.EmbeddingService.JinaConfigPath)
	if err != nil {
		appLogger.Error("failed to prepare jina runtime", err, "config_path", cfg.EmbeddingService.JinaConfigPath)
		util.Fatalf("failed to prepare jina runtime: %v", err)
	}
	defer cleanupRuntimeConfig()
	if err := validateJinaRuntimeConfig(runtimeConfigPath); err != nil {
		appLogger.Error("invalid jina runtime config", err, "config_path", runtimeConfigPath)
		util.Fatalf("invalid jina runtime config: %v", err)
	}

	jinaAdapter, err := cgo.NewJinaAdapter(runtimeConfigPath, appLogger)
	if err != nil {
		appLogger.Error("failed to initialize Jina adapter", err, "config_path", runtimeConfigPath)
		util.Fatalf("failed to initialize Jina adapter: %v", err)
	}
	defer jinaAdapter.Close()

	embeddingService := grpcAdapter.NewEmbeddingService(appLogger, jinaAdapter)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.EmbeddingService.Port))
	if err != nil {
		appLogger.Error("failed to listen grpc port", err, "port", cfg.EmbeddingService.Port)
		util.Fatalf("failed to listen grpc port %s: %v", cfg.EmbeddingService.Port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterDeepLearningServiceServer(grpcServer, embeddingService)
	reflection.Register(grpcServer)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	topics := cfg.EmbeddingService.Topics
	topics.BatchTextRequest = strings.TrimSpace(topics.BatchTextRequest)
	topics.BatchTextGroup = strings.TrimSpace(topics.BatchTextGroup)
	topics.BatchTextResult = strings.TrimSpace(topics.BatchTextResult)
	topics.BatchImageRequest = strings.TrimSpace(topics.BatchImageRequest)
	topics.BatchImageGroup = strings.TrimSpace(topics.BatchImageGroup)
	topics.BatchImageResult = strings.TrimSpace(topics.BatchImageResult)
	if topics.BatchTextRequest == "" || topics.BatchTextGroup == "" {
		util.Fatalf("embedding batch text topic/group is empty")
	}
	if topics.BatchImageRequest == "" || topics.BatchImageGroup == "" {
		util.Fatalf("embedding batch image topic/group is empty")
	}
	appLogger.Info(
		"embedding topic config",
		"batch_text_request", fmt.Sprintf("%q", topics.BatchTextRequest),
		"batch_text_group", fmt.Sprintf("%q", topics.BatchTextGroup),
		"batch_image_request", fmt.Sprintf("%q", topics.BatchImageRequest),
		"batch_image_group", fmt.Sprintf("%q", topics.BatchImageGroup),
	)

	batchTextErrCh := consumer.Start(rootCtx, topics.BatchTextRequest, topics.BatchTextGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		startedAt := time.Now()
		var request struct {
			Texts []string `json:"texts"`
		}
		if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
			appLogger.Error("invalid batch text payload", err, "topic", msg.Topic, "offset", msg.Offset)
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
			appLogger.Error("batch text embedding failed", runErr, "topic", msg.Topic, "offset", msg.Offset)
		}

		if topics.BatchTextResult != "" {
			publishErr := producer.PublishJSON(ctx, topics.BatchTextResult, msg.Message.Key, map[string]any{
				"status":     status,
				"message":    message,
				"count":      len(request.Texts),
				"dimension":  dimension,
				"embeddings": embeddings,
				"at":         time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic": topics.BatchTextRequest,
			})
			if publishErr != nil {
				appLogger.Error("publish batch text result failed", publishErr, "result_topic", topics.BatchTextResult)
			}
		}
		appLogger.Info(
			"batch text pipeline completed",
			"topic", msg.Topic,
			"status", status,
			"count", len(request.Texts),
			"dimension", dimension,
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)

		return nil
	})

	batchImageErrCh := consumer.Start(rootCtx, topics.BatchImageRequest, topics.BatchImageGroup, func(ctx context.Context, msg ports.ConsumeMessage) error {
		startedAt := time.Now()
		var request struct {
			Images   [][]byte `json:"images"`
			Width    int      `json:"width"`
			Height   int      `json:"height"`
			Channels int      `json:"channels"`
		}
		if err := json.Unmarshal(msg.Message.Value, &request); err != nil {
			appLogger.Error("invalid batch image payload", err, "topic", msg.Topic, "offset", msg.Offset)
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
			appLogger.Error("batch image embedding failed", runErr, "topic", msg.Topic, "offset", msg.Offset)
		}

		if topics.BatchImageResult != "" {
			publishErr := producer.PublishJSON(ctx, topics.BatchImageResult, msg.Message.Key, map[string]any{
				"status":     status,
				"message":    message,
				"count":      len(request.Images),
				"dimension":  dimension,
				"embeddings": embeddings,
				"at":         time.Now().UTC().Format(time.RFC3339),
			}, map[string]string{
				"source_topic": topics.BatchImageRequest,
			})
			if publishErr != nil {
				appLogger.Error("publish batch image result failed", publishErr, "result_topic", topics.BatchImageResult)
			}
		}
		appLogger.Info(
			"batch image pipeline completed",
			"topic", msg.Topic,
			"status", status,
			"count", len(request.Images),
			"dimension", dimension,
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)

		return nil
	})

	grpcErrCh := make(chan error, 1)
	go func() {
		appLogger.Info("embedding grpc server listening", "port", cfg.EmbeddingService.Port)
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			grpcErrCh <- serveErr
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		appLogger.Info("received shutdown signal")
	case err := <-grpcErrCh:
		if err != nil {
			appLogger.Error("grpc server stopped with error", err)
		}
	case err := <-batchTextErrCh:
		if err != nil {
			appLogger.Error("batch text consumer stopped with error", err)
		}
	case err := <-batchImageErrCh:
		if err != nil {
			appLogger.Error("batch image consumer stopped with error", err)
		}
	}

	cancel()
	waitConsumerStopped(batchTextErrCh, "batch_text", appLogger, 15*time.Second)
	waitConsumerStopped(batchImageErrCh, "batch_image", appLogger, 15*time.Second)
	grpcServer.GracefulStop()
	appLogger.Info("service stopped")
}

type jinaRuntimeConfig struct {
	Models struct {
		JinaTextEncoder struct {
			ModelPath string `yaml:"model_path"`
			VocabPath string `yaml:"vocab_path"`
		} `yaml:"jina_text_encoder"`
		JinaVisionEncoder struct {
			ModelPath string `yaml:"model_path"`
		} `yaml:"jina_vision_encoder"`
	} `yaml:"models"`
}

func prepareJinaRuntime(rawConfigPath string) (string, func(), error) {
	absConfigPath, err := filepath.Abs(strings.TrimSpace(rawConfigPath))
	if err != nil {
		return "", func() {}, err
	}
	configDir := filepath.Dir(absConfigPath)
	runtimeRoot := filepath.Clean(filepath.Join(configDir, ".."))
	if _, err := os.Stat(filepath.Join(runtimeRoot, "model")); err != nil {
		runtimeRoot = configDir
	}

	raw, err := os.ReadFile(absConfigPath)
	if err != nil {
		return "", func() {}, err
	}

	cfg := map[interface{}]interface{}{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return "", func() {}, err
	}

	if p, ok := getNestedString(cfg, "models", "jina_text_encoder", "model_path"); ok {
		_ = setNestedString(cfg, resolveMaybeRelative(runtimeRoot, p), "models", "jina_text_encoder", "model_path")
	}
	if p, ok := getNestedString(cfg, "models", "jina_text_encoder", "vocab_path"); ok {
		_ = setNestedString(cfg, resolveMaybeRelative(runtimeRoot, p), "models", "jina_text_encoder", "vocab_path")
	}
	if p, ok := getNestedString(cfg, "models", "jina_vision_encoder", "model_path"); ok {
		_ = setNestedString(cfg, resolveMaybeRelative(runtimeRoot, p), "models", "jina_vision_encoder", "model_path")
	}

	normalized, err := yaml.Marshal(cfg)
	if err != nil {
		return "", func() {}, err
	}

	tmpFile, err := os.CreateTemp("", "jina-runtime-config-*.yaml")
	if err != nil {
		return "", func() {}, err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(normalized); err != nil {
		return "", func() {}, err
	}

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanup, nil
}

func resolveMaybeRelative(baseDir string, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(baseDir, p))
}

func getNestedString(root map[interface{}]interface{}, keys ...string) (string, bool) {
	var current interface{} = root
	for _, key := range keys {
		m, ok := current.(map[interface{}]interface{})
		if !ok {
			return "", false
		}
		current, ok = m[key]
		if !ok {
			return "", false
		}
	}
	value, ok := current.(string)
	if !ok {
		return "", false
	}
	return value, true
}

func setNestedString(root map[interface{}]interface{}, value string, keys ...string) bool {
	if len(keys) == 0 {
		return false
	}
	var current interface{} = root
	for i := 0; i < len(keys)-1; i++ {
		m, ok := current.(map[interface{}]interface{})
		if !ok {
			return false
		}
		next, ok := m[keys[i]]
		if !ok {
			return false
		}
		current = next
	}
	m, ok := current.(map[interface{}]interface{})
	if !ok {
		return false
	}
	m[keys[len(keys)-1]] = value
	return true
}

func validateJinaRuntimeConfig(configPath string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read runtime config: %w", err)
	}

	cfg := jinaRuntimeConfig{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse runtime config: %w", err)
	}

	required := map[string]string{
		"jina_text_model_path":   cfg.Models.JinaTextEncoder.ModelPath,
		"jina_text_vocab_path":   cfg.Models.JinaTextEncoder.VocabPath,
		"jina_vision_model_path": cfg.Models.JinaVisionEncoder.ModelPath,
	}
	for key, p := range required {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("%s is empty in runtime config", key)
		}
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("%s not found at %s (hint: build/export third_party/onnx_c++ assets)", key, p)
		}
	}

	candidateRoots := resolveOnnxRuntimeRoots(required)
	foundBuildArtifacts := false
	for _, root := range candidateRoots {
		if hasEdgeSentinelBuildArtifacts(root) {
			foundBuildArtifacts = true
			break
		}
	}
	if !foundBuildArtifacts {
		return fmt.Errorf(
			"missing edge_sentinel build artifacts from runtime paths (searched roots=%v; hint: run cmake -S third_party/onnx_c++ -B build-debug && cmake --build build-debug)",
			candidateRoots,
		)
	}

	return nil
}

func resolveOnnxRuntimeRoots(required map[string]string) []string {
	roots := make([]string, 0)
	seen := make(map[string]struct{})
	for _, p := range required {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		current := filepath.Dir(abs)
		for {
			if filepath.Base(current) == "onnx_c++" {
				if _, ok := seen[current]; !ok {
					seen[current] = struct{}{}
					roots = append(roots, current)
				}
			}
			candidate := filepath.Join(current, "third_party", "onnx_c++")
			if _, err := os.Stat(candidate); err == nil {
				if _, ok := seen[candidate]; !ok {
					seen[candidate] = struct{}{}
					roots = append(roots, candidate)
				}
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return roots
}

func hasEdgeSentinelBuildArtifacts(onnxRoot string) bool {
	patterns := []string{
		filepath.Join(onnxRoot, "build", "src", "libedge_sentinel_lib.*"),
		filepath.Join(onnxRoot, "build-debug", "src", "libedge_sentinel_lib.*"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

func waitConsumerStopped(errCh <-chan error, consumerName string, logger util.Logger, timeout time.Duration) {
	if errCh == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case err, ok := <-errCh:
			if !ok {
				logger.Info("consumer stopped", "consumer", consumerName)
				return
			}
			if err != nil {
				logger.Error("consumer stopped with error", err, "consumer", consumerName)
			}
		case <-timer.C:
			logger.Error("consumer stop timeout", fmt.Errorf("timeout waiting for shutdown"), "consumer", consumerName)
			return
		}
	}
}
