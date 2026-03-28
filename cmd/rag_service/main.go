package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"strconv"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	"rag_imagetotext_texttoimage/internal/application/use_cases"
	infraQdrant "rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

func main() {
	config_loader := util.NewConfigLoader(
		"./config/.env",
		"./config/config.yaml",
	)
	config_loader.Load()
	config := config_loader.GetConfig()

	appLogger, err := util.NewFileLogger(config.Qdrant.LogPath, slog.LevelInfo)
	if err != nil {
		log.Fatalf("Not able to create logger: %v", err)
	}

	portInt, err := strconv.Atoi(config.Qdrant.Port)
	if err != nil {
		panic("Port không hợp lệ: " + err.Error())
	}

	config_qdrant := infraQdrant.Config{
		Host: config.Qdrant.Bootstrap,
		Port: portInt,
	}

	client, err := infraQdrant.NewClient(
		config_qdrant,
		appLogger,
	)
	if err != nil {
		appLogger.Error("Not able to create qdrant client", err)
		return
	}

	pointStore := infraQdrant.NewPointStore(client.Raw(), appLogger)
	collectionStore := infraQdrant.NewCollectionStore(client.Raw(), appLogger)

	searchWithVectorDB := usecases.NewSearchWithVectorDB(appLogger, pointStore)

	ragService := grpcAdapter.NewRagService(
		appLogger,
		searchWithVectorDB,
		pointStore,
		collectionStore,
	)
	port := "50051"
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		appLogger.Error("Failed to listen on TCP port", err)
		log.Fatalf("Failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterRagServiceServer(grpcServer, ragService)

	reflection.Register(grpcServer)
	go func() {
		appLogger.Info("gRPC server listening", "port", port)
		if err := grpcServer.Serve(lis); err != nil {
			appLogger.Error("Failed to serve gRPC server", err)
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	appLogger.Info("Shutting down gRPC server gracefully...")
	grpcServer.GracefulStop()
	appLogger.Info("Server exited.")
	


}
