package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	usecases "rag_imagetotext_texttoimage/internal/application/use_cases"
	infraQdrant "rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

func main() {
	configLoader := util.NewConfigLoader(
		"./config/.env",
		"./config/config.yaml",
	)
	if _, err := configLoader.Load(); err != nil {
		util.Fatalf("failed to load configuration: %v", err)
	}
	cfg := configLoader.GetConfig()

	appLogger, err := util.NewFileLogger(cfg.RAGService.LogPath, slog.LevelInfo)
	if err != nil {
		util.Fatalf("not able to create logger: %v", err)
	}
	defer appLogger.Close()
	appLogger.Info("rag service bootstrap started", "grpc_port", cfg.RAGService.Port, "qdrant_host", cfg.RAGService.QdrantHost, "qdrant_port", cfg.RAGService.QdrantPort, "log_path", cfg.RAGService.LogPath)

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
	collectionStore := infraQdrant.NewCollectionStore(client.Raw(), appLogger)

	searchWithVectorDB := usecases.NewSearchWithVectorDB(appLogger, pointStore)

	ragService := grpcAdapter.NewRagService(
		appLogger,
		searchWithVectorDB,
		pointStore,
		collectionStore,
	)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.RAGService.Port))
	if err != nil {
		appLogger.Error("listen tcp failed", err, "port", cfg.RAGService.Port)
		util.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	appLogger.Info("grpc graceful shutdown")
	grpcServer.GracefulStop()
	appLogger.Info("rag service stopped")

}
