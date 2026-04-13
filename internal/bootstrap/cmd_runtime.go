package bootstrap

import (
	"fmt"
	"log/slog"
	"strings"

	"rag_imagetotext_texttoimage/internal/util"
)

type CmdRuntimeOptions struct {
	Namespace      string
	EnvPath        string
	YamlPath       string
	LogLevel       slog.Level
	ResolveLogPath func(*util.Config) string
}

func BuildConfigAndLogger(opts CmdRuntimeOptions) (*util.Config, util.Logger, error) {
	container := NewContainer()
	registry := NewRegistry(container, opts.Namespace)
	configKey := registry.Key("config")
	loggerKey := registry.Key("logger")

	if err := registry.RegisterSingleton(configKey, func(_ Resolver) (any, error) {
		configLoader := util.NewConfigLoader(strings.TrimSpace(opts.EnvPath), strings.TrimSpace(opts.YamlPath))
		return configLoader.Load()
	}); err != nil {
		return nil, nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger register config failed: %w", err)
	}

	if err := registry.RegisterSingleton(loggerKey, func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		if opts.ResolveLogPath == nil {
			return nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger ResolveLogPath is nil")
		}
		logPath := strings.TrimSpace(opts.ResolveLogPath(cfg))
		if logPath == "" {
			return nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger resolved log path is empty")
		}
		return util.NewFileLogger(logPath, opts.LogLevel)
	}); err != nil {
		return nil, nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger register logger failed: %w", err)
	}

	cfg, err := ResolveAs[*util.Config](container, configKey)
	if err != nil {
		return nil, nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger resolve config failed: %w", err)
	}
	logger, err := ResolveAs[util.Logger](container, loggerKey)
	if err != nil {
		return nil, nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger resolve logger failed: %w", err)
	}
	return cfg, logger, nil
}
