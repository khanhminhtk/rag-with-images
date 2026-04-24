package bootstrap

import (
	"fmt"
	"log/slog"

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

	if err := registerCmdRuntimeBindings(container, cmdRuntimeBindingKeys{
		ConfigKey:      configKey,
		LoggerKey:      loggerKey,
		EnvPath:        opts.EnvPath,
		YamlPath:       opts.YamlPath,
		LogLevel:       opts.LogLevel,
		ResolveLogPath: opts.ResolveLogPath,
	}); err != nil {
		return nil, nil, fmt.Errorf("internal.bootstrap.BuildConfigAndLogger register bindings failed: %w", err)
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
