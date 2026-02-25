package util

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Qdrant QdrantConfig `yaml:"qdrant"`
}

type QdrantConfig struct {
	Bootstrap string `yaml:"bootstrap"`
	Port      string `yaml:"port"`
	UseGRPC   bool   `yaml:"use_gRPC"`
}

type ConfigLoader interface {
	LoadConfig(filePath string) (*Config, error)
}

type YAMLConfig struct {
}

func NewYAMLConfig() *YAMLConfig {
	return &YAMLConfig{}
}

func (Y *YAMLConfig) LoadConfig(filePath string) (*Config, error) {
	slog.Info("Starting to load configuration", "path", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("Failed to open config file", "error", err, "path", filePath)
		return nil, fmt.Errorf("os.Open failed: %w", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			fmt.Printf("Can't close file %s: %v\n", filePath, err)
		}
	}()
	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		slog.Error("Failed to decode YAML syntax", "error", err)
		return nil, fmt.Errorf("yaml.Decode failed: %w", err)
	}
	return &cfg, nil
}
