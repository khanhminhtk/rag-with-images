package main

import (
	"context"
	"fmt"
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
		util.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Close()

	appLogger.Info("llm service startup")

	envPath := "config/.env"
	yamlPath := "config/config.yaml"
	configLoader := util.NewConfigLoader(envPath, yamlPath)
	cfg, err := configLoader.Load()
	if err != nil {
		appLogger.Error("load configuration failed", err)
		util.Fatalf("failed to load configuration: %v", err)
	}
	appLogger.Info("load configuration success")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	geminiClient, err := llm.NewGemini(cfg.LLMService, ctx, appLogger)
	if err != nil {
		appLogger.Error("initialize gemini client failed", err)
		util.Fatalf("failed to initialize gemini client: %v", err)
	}

	llmServiceServer := grpcAdapter.NewLLMService(appLogger, geminiClient)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.LLMService.Port))
	if err != nil {
		appLogger.Error("listen tcp failed", err, "port", cfg.LLMService.Port)
		util.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterLlmServiceServer(grpcServer, llmServiceServer)

	reflection.Register(grpcServer)

	go func() {
		appLogger.Info("gRPC server listening", "port", cfg.LLMService.Port)
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
	appLogger.Info("llm service stopped")
}
