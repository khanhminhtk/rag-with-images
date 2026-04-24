package bootstrap

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"rag_imagetotext_texttoimage/internal/util"
)

func registerServiceConfigAndLogger(container *DIContainer, configKey, loggerKey, logPath string, level slog.Level) error {
	if err := registerSingleton(container, configKey, getConfig(ProjectPath("config", ".env"), ProjectPath("config", "config.yaml"))); err != nil {
		return err
	}
	if err := registerSingleton(container, loggerKey, getFileLogger(logPath, level)); err != nil {
		return err
	}
	return nil
}

func resolveServiceConfigAndLogger(r Resolver, configKey, loggerKey string) (*util.Config, util.Logger, error) {
	cfg, err := ResolveAs[*util.Config](r, configKey)
	if err != nil {
		return nil, nil, err
	}
	logger, err := ResolveAs[util.Logger](r, loggerKey)
	if err != nil {
		return nil, nil, err
	}
	return cfg, logger, nil
}

func newServiceMetricsHTTPServer(host, port, defaultPort string) *http.Server {
	resolvedHost := strings.TrimSpace(host)
	if resolvedHost == "" {
		resolvedHost = "0.0.0.0"
	}
	resolvedPort := strings.TrimSpace(port)
	if resolvedPort == "" {
		resolvedPort = defaultPort
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:    fmt.Sprintf("%s:%s", resolvedHost, resolvedPort),
		Handler: mux,
	}
}
