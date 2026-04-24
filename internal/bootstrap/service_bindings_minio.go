package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	useCaseMinio "rag_imagetotext_texttoimage/internal/application/use_cases/minio"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	infraMinio "rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type minioBindingKeys struct {
	ConfigKey        string
	LoggerKey        string
	RuntimeBucketKey string
	StorageKey       string
	UseCasesKey      string
	TopicsKey        string
	KafkaInfraKey    string
	GRPCServiceKey   string
	GRPCServerKey    string
	ListenerKey      string
	MetricsHTTPKey   string
}

func registerMinioBindings(container *DIContainer, keys minioBindingKeys) error {
	if err := registerMinioConfigAndLogger(container, keys); err != nil {
		return err
	}
	if err := registerMinioStorageAndUseCases(container, keys); err != nil {
		return err
	}
	if err := registerMinioTopicsAndKafka(container, keys); err != nil {
		return err
	}
	if err := registerMinioGRPC(container, keys); err != nil {
		return err
	}
	if err := registerMinioMetricsHTTP(container, keys); err != nil {
		return err
	}
	return nil
}

func registerMinioConfigAndLogger(container *DIContainer, keys minioBindingKeys) error {
	return registerServiceConfigAndLogger(container, keys.ConfigKey, keys.LoggerKey, ProjectPath("logs", "minio_service.log"), slog.LevelInfo)
}

func registerMinioStorageAndUseCases(container *DIContainer, keys minioBindingKeys) error {
	if err := registerSingleton(container, keys.RuntimeBucketKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		runtimeBucket := cfg.MinIOService.Bucket("default")
		if runtimeBucket == "" {
			runtimeBucket = cfg.MinIOService.DefaultBucket
		}
		if runtimeBucket == "" {
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioStorageAndUseCases failed: minio runtime bucket is empty")
		}
		return runtimeBucket, nil
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.StorageKey, func(r Resolver) (any, error) {
		cfg, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
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
			logger.Error("internal.bootstrap.service_bindings_minio.registerMinioStorageAndUseCases failed to create minio client", err)
			return nil, err
		}
		storage := infraMinio.NewMinIOStorage(*minioClient, logger)
		runtimeBucket, err := ResolveAs[string](r, keys.RuntimeBucketKey)
		if err != nil {
			return nil, err
		}
		if err := storage.EnsureBucket(context.Background(), runtimeBucket); err != nil {
			logger.Error("internal.bootstrap.service_bindings_minio.registerMinioStorageAndUseCases failed to ensure bucket", err, "bucket", runtimeBucket)
			return nil, err
		}
		return storage, nil
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.UseCasesKey, func(r Resolver) (any, error) {
		cfg, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
		if err != nil {
			return nil, err
		}
		runtimeBucket, err := ResolveAs[string](r, keys.RuntimeBucketKey)
		if err != nil {
			return nil, err
		}
		storage, err := ResolveAs[*infraMinio.MinIOStorage](r, keys.StorageKey)
		if err != nil {
			return nil, err
		}
		deleteUC := (&useCaseMinio.DeleteFileInputUseCase{}).NewDeleteFileInputUseCase(runtimeBucket, logger, storage)
		presignUC := (&useCaseMinio.PresignGetObjectUseCase{}).NewPresignGetObjectUseCase(runtimeBucket, time.Duration(cfg.MinIOService.PresignExpiryS)*time.Second, logger, storage)
		uploadUC := useCaseMinio.NewUploadLocalFileToMinIOUseCase(runtimeBucket, logger, storage)
		return minioUseCases{deleteUC: deleteUC, presignUC: presignUC, uploadUC: uploadUC}, nil
	}); err != nil {
		return err
	}
	return nil
}

func registerMinioTopicsAndKafka(container *DIContainer, keys minioBindingKeys) error {
	if err := registerSingleton(container, keys.TopicsKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		topics := cfg.MinIOService.Topics
		topics.UploadRequest = strings.TrimSpace(topics.UploadRequest)
		topics.UploadGroup = strings.TrimSpace(topics.UploadGroup)
		topics.UploadResult = strings.TrimSpace(topics.UploadResult)
		if topics.UploadRequest == "" {
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioTopicsAndKafka failed: minio upload request topic is empty")
		}
		if topics.UploadGroup == "" {
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioTopicsAndKafka failed: minio upload group is empty")
		}
		return topics, nil
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
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioTopicsAndKafka failed to create kafka adapters: %w", err)
		}
		return minioKafkaInfra{producer: producer, consumer: consumer}, nil
	}); err != nil {
		return err
	}
	return nil
}

func registerMinioGRPC(container *DIContainer, keys minioBindingKeys) error {
	if err := registerSingleton(container, keys.GRPCServiceKey, func(r Resolver) (any, error) {
		_, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
		if err != nil {
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioGRPC failed to resolve service config and logger: %w", err)
		}
		ucs, err := ResolveAs[minioUseCases](r, keys.UseCasesKey)
		if err != nil {
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioGRPC failed to resolve use cases: %w", err)
		}
		return grpcAdapter.NewMinioService(ucs.deleteUC, ucs.presignUC, logger), nil
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.GRPCServerKey, func(r Resolver) (any, error) {
		service, err := ResolveAs[*grpcAdapter.MinioService](r, keys.GRPCServiceKey)
		if err != nil {
			return nil, fmt.Errorf("internal.bootstrap.service_bindings_minio.registerMinioGRPC failed to resolve grpc service: %w", err)
		}
		_, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
		if err != nil {
			return nil, err
		}
		grpcServer := grpc.NewServer()
		pb.RegisterMinioServiceServer(grpcServer, service)
		reflection.Register(grpcServer)
		logger.Info("minio grpc reflection enabled")
		return grpcServer, nil
	}); err != nil {
		return err
	}
	if err := registerSingleton(container, keys.ListenerKey, func(r Resolver) (any, error) {
		cfg, logger, err := resolveServiceConfigAndLogger(r, keys.ConfigKey, keys.LoggerKey)
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
		return err
	}
	return nil
}

func registerMinioMetricsHTTP(container *DIContainer, keys minioBindingKeys) error {
	if err := registerSingleton(container, keys.MetricsHTTPKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, keys.ConfigKey)
		if err != nil {
			return nil, err
		}
		return newServiceMetricsHTTPServer(cfg.MinIOService.IDMonitoring, cfg.MinIOService.PortMetricGRPC, "9104"), nil
	}); err != nil {
		return err
	}
	return nil
}
