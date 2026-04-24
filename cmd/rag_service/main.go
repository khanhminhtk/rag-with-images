package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	usecases "rag_imagetotext_texttoimage/internal/application/use_cases"
	"rag_imagetotext_texttoimage/internal/bootstrap"
	infraQdrant "rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

func main() {
	cfg, appLogger, err := bootstrap.BuildConfigAndLogger(bootstrap.CmdRuntimeOptions{
		Namespace: "rag_service",
		EnvPath:   "./config/.env",
		YamlPath:  "./config/config.yaml",
		LogLevel:  slog.LevelInfo,
		ResolveLogPath: func(_ *util.Config) string {
			return "logs/rag_service.log"
		},
	})
	if err != nil {
		util.Fatalf("failed to bootstrap rag runtime: %v", err)
	}
	defer appLogger.Close()
	appLogger.Info("rag service bootstrap started", "grpc_port", cfg.RAGService.Port, "qdrant_host", cfg.RAGService.QdrantHost, "qdrant_port", cfg.RAGService.QdrantPort, "log_path", "logs/rag_service.log")

	portInt, err := strconv.Atoi(cfg.RAGService.QdrantPort)
	if err != nil {
		util.Fatalf("invalid qdrant port %s: %v", cfg.RAGService.QdrantPort, err)
	}

	configQdrant := infraQdrant.Config{
		Host: cfg.RAGService.QdrantHost,
		Port: portInt,
	}

	client, err := infraQdrant.NewClient(
		configQdrant,
		appLogger,
	)
	if err != nil {
		appLogger.Error("create qdrant client failed", err)
		return
	}
	appLogger.Info("rag service qdrant client ready")

	pointStore := infraQdrant.NewPointStore(client.Raw(), appLogger)
	collectionStore := infraQdrant.NewCollectionStore(
		client.Raw(),
		appLogger,
		infraQdrant.WithQdrantRequestTimeout(time.Duration(cfg.RAGService.QdrantRequestTimeoutSeconds)*time.Second),
		infraQdrant.WithQdrantRetryAttempts(cfg.RAGService.QdrantRetryAttempts),
		infraQdrant.WithQdrantRetryBackoff(time.Duration(cfg.RAGService.QdrantRetryBackoffMs)*time.Millisecond),
	)

	searchWithVectorDB := usecases.NewSearchWithVectorDB(appLogger, pointStore)

	ragService := grpcAdapter.NewRagService(
		appLogger,
		searchWithVectorDB,
		pointStore,
		collectionStore,
	)

	startGRPCRagService(appLogger, *cfg, ragService)
	appLogger.Info("rag service stopped")

}

func startGRPCRagService(appLogger util.Logger, cfg util.Config, ragService *grpcAdapter.RagService) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.RAGService.Port))
	if err != nil {
		appLogger.Error("listen tcp failed", err, "port", cfg.RAGService.Port)
		util.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
	)
	pb.RegisterRagServiceServer(grpcServer, ragService)

	reflection.Register(grpcServer)
	appLogger.Info("rag service grpc reflection enabled")
	go func() {
		appLogger.Info("grpc server listening", "port", cfg.RAGService.Port)
		if err := grpcServer.Serve(lis); err != nil {
			appLogger.Error("grpc serve failed", err)
			util.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		metricsAddr := fmt.Sprintf("%s:%s", cfg.RAGService.IDMonitoring, cfg.RAGService.PortMetricGRPC)
		appLogger.Info("Prometheus metrics server listening on " + metricsAddr + "/metrics")
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			appLogger.Error("failed to serve metrics: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	appLogger.Info("grpc graceful shutdown")
	grpcServer.GracefulStop()
	appLogger.Info("rag service stopped")
}
