package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	"rag_imagetotext_texttoimage/internal/infra/llm"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

func main() {
	appLogger, err := util.NewFileLogger("logs/llm_service.log", slog.LevelDebug)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer appLogger.Close()

	appLogger.Info("Starting LLM gRPC Service...")

	envPath := "config/.env"
	yamlPath := "config/config.yaml"
	configLoader := util.NewConfigLoader(envPath, yamlPath)
	cfg, err := configLoader.Load()
	if err != nil {
		appLogger.Error("Failed to load configuration", err)
		log.Fatalf("Failed to load configuration: %v", err)
	}
	appLogger.Info("Configuration loaded successfully")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	geminiClient, err := llm.NewGemini(cfg.LLM, ctx, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize Gemini Client", err)
		log.Fatalf("Failed to initialize Gemini Client: %v", err)
	}

	llmServiceServer := grpcAdapter.NewLLMService(appLogger, geminiClient)

	port := "50051"
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		appLogger.Error("Failed to listen on TCP port", err)
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterLlmServiceServer(grpcServer, llmServiceServer)

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
