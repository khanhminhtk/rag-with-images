package util

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Qdrant QdrantConfig `yaml:"qdrant"`
	LLM    LLMConfig    `yaml:"llm"`
}

type LLMConfig struct {
	Model  string  `yaml:"model"`
	Temp   float32 `yaml:"temp"`
	ApiKey string  `yaml:"apikey"`
}

type QdrantConfig struct {
	Bootstrap string `yaml:"bootstrap"`
	Port      string `yaml:"port"`
	UseGRPC   bool   `yaml:"use_gRPC"`
}
type ConfigLoader struct {
	envPath  string
	yamlPath string
	config   *Config
}

func NewConfigLoader(envPath string, yamlPath string) *ConfigLoader {
	return &ConfigLoader{
		envPath:  envPath,
		yamlPath: yamlPath,
		config:   &Config{},
	}
}

func (c *ConfigLoader) Load() (*Config, error) {
	if err := c.loadEnv(); err != nil {
		return nil, err
	}

	if err := c.loadYaml(); err != nil {
		return nil, err
	}

	return c.config, nil
}

func (c *ConfigLoader) loadEnv() error {
	slog.Info("Loading ENV variables", "path", c.envPath)
	err := godotenv.Load(c.envPath)
	if err != nil {
		slog.Warn("Could not load .env file, relying on system env variables")
	}
	return nil
}

func (c *ConfigLoader) loadYaml() error {
	slog.Info("Loading YAML config", "path", c.yamlPath)

	yamlBytes, err := os.ReadFile(c.yamlPath)
	if err != nil {
		return fmt.Errorf("failed to read yaml file: %w", err)
	}

	expandedYAML := os.ExpandEnv(string(yamlBytes))

	if err := yaml.Unmarshal([]byte(expandedYAML), c.config); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}

	return nil
}

func (c *ConfigLoader) GetConfig() *Config {
	return c.config
}
