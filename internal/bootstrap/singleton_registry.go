package bootstrap

import (
	"fmt"
	"log/slog"
	"strings"

	"rag_imagetotext_texttoimage/internal/util"
)

func registerSingleton(container *DIContainer, key string, constructor Constructor) error {
	if container == nil {
		return fmt.Errorf("internal.bootstrap.registerSingleton container is nil")
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("internal.bootstrap.registerSingleton key is empty")
	}
	if constructor == nil {
		return fmt.Errorf("internal.bootstrap.registerSingleton constructor is nil for key %s", key)
	}

	if err := container.RegisterSingleton(key, constructor); err != nil {
		return fmt.Errorf("internal.bootstrap.registerSingleton register key %s failed: %w", key, err)
	}
	return nil
}

func getConfig(envPath string, yamlPath string) Constructor {
	return func(_ Resolver) (any, error) {
		configLoader := util.NewConfigLoader(strings.TrimSpace(envPath), strings.TrimSpace(yamlPath))
		return configLoader.Load()
	}
}

func getFileLogger(logPath string, level slog.Level) Constructor {
	return func(_ Resolver) (any, error) {
		return util.NewFileLogger(strings.TrimSpace(logPath), level)
	}
}

func getLoggerFromConfig(configKey string, level slog.Level, resolveLogPath func(*util.Config) string) Constructor {
	return func(r Resolver) (any, error) {
		cfg, err := ResolveAs[*util.Config](r, configKey)
		if err != nil {
			return nil, err
		}
		if resolveLogPath == nil {
			return nil, fmt.Errorf("internal.bootstrap.getLoggerFromConfig ResolveLogPath is nil")
		}
		logPath := strings.TrimSpace(resolveLogPath(cfg))
		if logPath == "" {
			return nil, fmt.Errorf("internal.bootstrap.getLoggerFromConfig resolved log path is empty")
		}
		return util.NewFileLogger(logPath, level)
	}
}
