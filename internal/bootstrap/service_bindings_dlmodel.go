package bootstrap

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/infra/cgo"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type dlmodelBindingKeys struct {
	ConfigKey      string
	LoggerKey      string
	JinaRuntimeKey string
	JinaAdapterKey string
	TopicsKey      string
	KafkaInfraKey  string
	GRPCServerKey  string
	ListenerKey    string
	MetricsHTTPKey string
}

func registerDLModelBindings(container *DIContainer, keys dlmodelBindingKeys) error {
	if err := registerDLModelConfigAndLogger(container, keys); err != nil {
		return err
	}
	if err := registerDLModelRuntime(container, keys); err != nil {
		return err
	}
	if err := registerDLModelTopicsAndKafka(container, keys); err != nil {
		return err
	}
	if err := registerDLModelGRPC(container, keys); err != nil {
		return err
	}
	if err := registerDLModelMetricsHTTP(container, keys); err != nil {
		return err
	}
	return nil
}

func registerDLModelConfigAndLogger(container *DIContainer, keys dlmodelBindingKeys) error {
	return registerServiceConfigAndLogger(container, keys.ConfigKey, keys.LoggerKey, ProjectPath("logs", "embedding_service.log"), slog.LevelInfo)
}

func registerDLModelRuntime(container *DIContainer, keys dlmodelBindingKeys) error {
	if err := registerSingleton(container, keys.JinaRuntimeKey, func(r Resolver) (any, error) {
		cfg, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
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
		return err
	}
	if err := registerSingleton(container, keys.JinaAdapterKey, func(r Resolver) (any, error) {
		_, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
		if err != nil {
			return nil, err
		}
		runtimeCfg, err := ResolveAs[*dlmodelJinaRuntimeConfig](r, keys.JinaRuntimeKey)
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
		return err
	}
	return nil
}

func registerDLModelTopicsAndKafka(container *DIContainer, keys dlmodelBindingKeys) error {
	if err := registerSingleton(container, keys.TopicsKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		return buildEmbeddingTopics(cfg.EmbeddingService.Topics)
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.KafkaInfraKey, func(r Resolver) (any, error) {
		cfg, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
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
		return err
	}
	return nil
}

func registerDLModelGRPC(container *DIContainer, keys dlmodelBindingKeys) error {
	if err := registerSingleton(container, keys.GRPCServerKey, func(r Resolver) (any, error) {
		_, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
		if err != nil {
			return nil, err
		}
		jina, err := ResolveAs[*cgo.JinaAdapter](r, keys.JinaAdapterKey)
		if err != nil {
			return nil, err
		}
		embeddingService := grpcAdapter.NewEmbeddingService(logger, jina)
		grpcServer := grpc.NewServer(
			grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
			grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
			grpc.MaxRecvMsgSize(30*1024*1024),
			grpc.MaxSendMsgSize(30*1024*1024),
		)
		pb.RegisterDeepLearningServiceServer(grpcServer, embeddingService)
		grpc_prometheus.EnableHandlingTimeHistogram()
		grpc_prometheus.Register(grpcServer)
		reflection.Register(grpcServer)
		logger.Info("embedding grpc reflection enabled")
		return grpcServer, nil
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.ListenerKey, func(r Resolver) (any, error) {
		cfg, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
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
		return err
	}
	return nil
}

func registerDLModelMetricsHTTP(container *DIContainer, keys dlmodelBindingKeys) error {
	if err := registerSingleton(container, keys.MetricsHTTPKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		return newServiceMetricsHTTPServer(cfg.EmbeddingService.IDMonitoring, cfg.EmbeddingService.PortMetricGRPC, "9103"), nil
	}); err != nil {
		return err
	}
	return nil
}
