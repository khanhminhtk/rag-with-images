package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	grpcAdapter "rag_imagetotext_texttoimage/internal/adapter/grpc"
	"rag_imagetotext_texttoimage/internal/bootstrap"
	"rag_imagetotext_texttoimage/internal/infra/llm"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

func main() {
	logPath := "logs/llm_service.log"
	cfg, appLogger, err := bootstrap.BuildConfigAndLogger(bootstrap.CmdRuntimeOptions{
		Namespace: "llm_service",
		EnvPath:   "config/.env",
		YamlPath:  "config/config.yaml",
		LogLevel:  slog.LevelDebug,
		ResolveLogPath: func(_ *util.Config) string {
			return logPath
		},
	})
	if err != nil {
		util.Fatalf("failed to bootstrap llm runtime: %v", err)
	}
	defer appLogger.Close()
	appLogger.Info("llm service bootstrap started", "env_path", "config/.env", "yaml_path", "config/config.yaml", "log_path", logPath)

	appLogger.Info("load configuration success")
	appLogger.Info("llm runtime config", "grpc_port", cfg.LLMService.Port, "model", cfg.LLMService.Model, "temperature", cfg.LLMService.Temp)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	geminiClient, err := llm.NewGemini(cfg.LLMService, ctx, appLogger)
	if err != nil {
		appLogger.Error("initialize gemini client failed", err)
		util.Fatalf("failed to initialize gemini client: %v", err)
	}
	appLogger.Info("gemini client ready")
	startGRPCLLMService(appLogger, *cfg, geminiClient)
	appLogger.Info("llm service stopped")
}

func startGRPCLLMService(appLogger util.Logger, cfg util.Config, gemini *llm.Gemini) {
	llmServiceServer := grpcAdapter.NewLLMService(appLogger, gemini)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.LLMService.Port))
	if err != nil {
		appLogger.Error("listen tcp failed", err, "port", cfg.LLMService.Port)
		util.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpc_prometheus.UnaryServerInterceptor),
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
	)
	
	pb.RegisterLlmServiceServer(grpcServer, llmServiceServer)
	grpc_prometheus.EnableHandlingTimeHistogram()
	grpc_prometheus.Register(grpcServer)

	reflection.Register(grpcServer)
	appLogger.Info("llm grpc reflection enabled")

	go func() {
		appLogger.Info("gRPC server listening", "port", cfg.LLMService.Port)
		if err := grpcServer.Serve(lis); err != nil {
			appLogger.Error("grpc serve failed", err)
			util.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		metricsAddr := fmt.Sprintf("%s:%s", cfg.LLMService.IdMonitoring, cfg.LLMService.PortMetricGRPC)
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
}