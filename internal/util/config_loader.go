package util

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Qdrant QdrantConfig `yaml:"qdrant"`
	LLM    LLMConfig    `yaml:"llm"`
	MinIO  MinIOConfig  `yaml:"minio"`
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
	LogPath   string `yaml:"log_path"`
}

type MinIOConfig struct {
	Endpoint       string            `yaml:"endpoint"`
	AccessKey      string            `yaml:"access_key"`
	SecretKey      string            `yaml:"secret_key"`
	UseSSL         bool              `yaml:"use_ssl"`
	DefaultBucket  string            `yaml:"default_bucket"`
	Buckets        map[string]string `yaml:"buckets"`
	Region         string            `yaml:"region"`
	PresignExpiryS int               `yaml:"presign_expiry_seconds"`
}

func (m MinIOConfig) Bucket(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || key == "default" {
		return strings.TrimSpace(m.DefaultBucket)
	}
	if m.Buckets == nil {
		return strings.TrimSpace(m.DefaultBucket)
	}
	bucket := strings.TrimSpace(m.Buckets[key])
	if bucket == "" {
		return strings.TrimSpace(m.DefaultBucket)
	}
	return bucket
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
	c.applyEnvOverrides()

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

func (c *ConfigLoader) applyEnvOverrides() {
	if c.config == nil {
		return
	}

	if c.config.MinIO.Buckets == nil {
		c.config.MinIO.Buckets = make(map[string]string)
	}

	defaultBucket := strings.TrimSpace(c.config.MinIO.DefaultBucket)
	if defaultBucket == "" {
		defaultBucket = strings.TrimSpace(os.Getenv("MINIO_DEFAULT_BUCKET"))
	}
	if defaultBucket != "" {
		c.config.MinIO.DefaultBucket = defaultBucket
		if strings.TrimSpace(c.config.MinIO.Buckets["default"]) == "" {
			c.config.MinIO.Buckets["default"] = defaultBucket
		}
	}

	for _, envKV := range os.Environ() {
		if !strings.HasPrefix(envKV, "MINIO_BUCKET_") {
			continue
		}
		parts := strings.SplitN(envKV, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimPrefix(parts[0], "MINIO_BUCKET_"))
		bucketName := strings.TrimSpace(parts[1])
		if key == "" || bucketName == "" {
			continue
		}
		c.config.MinIO.Buckets[key] = bucketName
	}

	rawBuckets := strings.TrimSpace(os.Getenv("MINIO_BUCKETS"))
	if rawBuckets != "" {
		for _, item := range strings.Split(rawBuckets, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			separator := "="
			if strings.Contains(item, ":") {
				separator = ":"
			}
			pair := strings.SplitN(item, separator, 2)
			if len(pair) != 2 {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(pair[0]))
			bucketName := strings.TrimSpace(pair[1])
			if key == "" || bucketName == "" {
				continue
			}
			c.config.MinIO.Buckets[key] = bucketName
		}
	}

	if c.config.MinIO.DefaultBucket == "" {
		if bucket := strings.TrimSpace(c.config.MinIO.Buckets["default"]); bucket != "" {
			c.config.MinIO.DefaultBucket = bucket
		}
	}
}

func (c *ConfigLoader) GetConfig() *Config {
	return c.config
}
