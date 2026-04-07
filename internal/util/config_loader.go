package util

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Qdrant           QdrantConfig         `yaml:"qdrant"`
	LLMService       LLMSettings          `yaml:"llm_service"`
	MinIOService     MinIOSettings        `yaml:"minio_service"`
	Kafka            KafkaConfig          `yaml:"kafka"`
	EmbeddingService EmbeddingSettings    `yaml:"embedding_service"`
	RAGService       RAGSettings          `yaml:"rag_service"`
	FileTraining     FileTrainingSettings `yaml:"process_file_service"`
}

type LLMSettings struct {
	Model  string  `yaml:"model"`
	Temp   float32 `yaml:"temp"`
	ApiKey string  `yaml:"apikey"`
	Port   string  `yaml:"port"`
}

type QdrantConfig struct {
	Bootstrap string `yaml:"bootstrap"`
	Port      string `yaml:"port"`
	UseGRPC   bool   `yaml:"use_gRPC"`
	LogPath   string `yaml:"log_path"`
}

type MinIOSettings struct {
	Endpoint       string            `yaml:"endpoint"`
	AccessKey      string            `yaml:"access_key"`
	SecretKey      string            `yaml:"secret_key"`
	UseSSL         bool              `yaml:"use_ssl"`
	DefaultBucket  string            `yaml:"default_bucket"`
	Buckets        map[string]string `yaml:"buckets"`
	Region         string            `yaml:"region"`
	PresignExpiryS int               `yaml:"presign_expiry_seconds"`
	GRPCPort       string            `yaml:"grpc_port"`
	Topics         MinIOTopics       `yaml:"topics"`
}

type MinIOTopics struct {
	UploadRequest string `yaml:"upload_request"`
	UploadGroup   string `yaml:"upload_group"`
	UploadResult  string `yaml:"upload_result"`
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
}

type EmbeddingSettings struct {
	Port           string          `yaml:"port"`
	LogPath        string          `yaml:"log_path"`
	JinaConfigPath string          `yaml:"jina_config_path"`
	Topics         EmbeddingTopics `yaml:"topics"`
}

type EmbeddingTopics struct {
	BatchTextRequest  string `yaml:"batch_text_request"`
	BatchTextGroup    string `yaml:"batch_text_group"`
	BatchTextResult   string `yaml:"batch_text_result"`
	BatchImageRequest string `yaml:"batch_image_request"`
	BatchImageGroup   string `yaml:"batch_image_group"`
	BatchImageResult  string `yaml:"batch_image_result"`
}

type RAGSettings struct {
	Port          string `yaml:"port"`
	LogPath       string `yaml:"log_path"`
	QdrantHost    string `yaml:"qdrant_host"`
	QdrantPort    string `yaml:"qdrant_port"`
	QdrantUseGRPC bool   `yaml:"qdrant_use_gRPC"`
	GRPCHost      string `yaml:"rag_grpc_host"`
	GRPCPort      string `yaml:"rag_grpc_port"`
}

type FileTrainingTopics struct {
	ProcessFileRequest string `yaml:"process_file_request"`
	ProcessFileGroup   string `yaml:"process_file_group"`
	ProcessFileResult  string `yaml:"process_file_result"`
}

type FileTrainingSettings struct {
	Port         string             `yaml:"port"`
	LogPath      string             `yaml:"log_path"`
	PathDownload string             `yaml:"path_download"`
	Topics       FileTrainingTopics `yaml:"topics"`
}

func (m MinIOSettings) Bucket(name string) string {
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
	resolvedEnvPath := resolveExistingPath(c.envPath)
	if resolvedEnvPath == "" {
		slog.Warn(withCaller(normalizeMessage("could not resolve .env file, relying on system env variables"), 1), "requested_path", c.envPath)
		return nil
	}

	slog.Info(withCaller(normalizeMessage("loading env variables"), 1), "path", resolvedEnvPath)
	err := godotenv.Load(resolvedEnvPath)
	if err != nil {
		slog.Warn(withCaller(normalizeMessage("could not load .env file, relying on system env variables"), 1), "path", resolvedEnvPath, "error", err)
	}
	return nil
}

func (c *ConfigLoader) loadYaml() error {
	resolvedYAMLPath := resolveExistingPath(c.yamlPath)
	if resolvedYAMLPath == "" {
		return fmt.Errorf("failed to resolve yaml path: %s", c.yamlPath)
	}

	slog.Info(withCaller(normalizeMessage("loading yaml config"), 1), "path", resolvedYAMLPath)

	yamlBytes, err := os.ReadFile(resolvedYAMLPath)
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

	if c.config.MinIOService.Buckets == nil {
		c.config.MinIOService.Buckets = make(map[string]string)
	}

	defaultBucket := strings.TrimSpace(c.config.MinIOService.DefaultBucket)
	if defaultBucket == "" {
		defaultBucket = strings.TrimSpace(os.Getenv("MINIO_DEFAULT_BUCKET"))
	}
	if defaultBucket != "" {
		c.config.MinIOService.DefaultBucket = defaultBucket
		if strings.TrimSpace(c.config.MinIOService.Buckets["default"]) == "" {
			c.config.MinIOService.Buckets["default"] = defaultBucket
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
		c.config.MinIOService.Buckets[key] = bucketName
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
			c.config.MinIOService.Buckets[key] = bucketName
		}
	}

	if c.config.MinIOService.DefaultBucket == "" {
		if bucket := strings.TrimSpace(c.config.MinIOService.Buckets["default"]); bucket != "" {
			c.config.MinIOService.DefaultBucket = bucket
		}
	}

	if c.config.RAGService.LogPath == "" {
		c.config.RAGService.LogPath = c.config.Qdrant.LogPath
	}
	if c.config.RAGService.QdrantHost == "" {
		c.config.RAGService.QdrantHost = c.config.Qdrant.Bootstrap
	}
	if c.config.RAGService.QdrantPort == "" {
		c.config.RAGService.QdrantPort = c.config.Qdrant.Port
	}
	if !c.config.RAGService.QdrantUseGRPC {
		c.config.RAGService.QdrantUseGRPC = c.config.Qdrant.UseGRPC
	}

	if v := firstNonEmptyEnv("RAG_SERVICE_LOG_PATH"); v != "" {
		c.config.RAGService.LogPath = v
	}
	if v := firstNonEmptyEnv("RAG_QDRANT_HOST", "RAG_SERVICE_QDRANT_HOST"); v != "" {
		c.config.RAGService.QdrantHost = v
	}
	if v := firstNonEmptyEnv("RAG_QDRANT_PORT", "RAG_SERVICE_QDRANT_PORT"); v != "" {
		c.config.RAGService.QdrantPort = v
	}
	if v := firstNonEmptyEnv("RAG_GRPC_HOST", "RAG_SERVICE_GRPC_HOST"); v != "" {
		c.config.RAGService.GRPCHost = v
	}
	if v := firstNonEmptyEnv("RAG_GRPC_PORT", "RAG_SERVICE_GRPC_PORT"); v != "" {
		c.config.RAGService.GRPCPort = v
	}
	if v := firstNonEmptyEnv("RAG_QDRANT_USE_GRPC", "RAG_SERVICE_QDRANT_USE_GRPC"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			c.config.RAGService.QdrantUseGRPC = parsed
		}
	}

	if strings.TrimSpace(c.config.RAGService.GRPCHost) == "" {
		c.config.RAGService.GRPCHost = "localhost"
	}
	if strings.TrimSpace(c.config.RAGService.GRPCPort) == "" {
		c.config.RAGService.GRPCPort = c.config.RAGService.Port
	}

	if v := firstNonEmptyEnv("GEMINI_API_KEY", "gemini_api_key", "LLM_API_KEY"); v != "" {
		c.config.LLMService.ApiKey = v
	}
	if v := firstNonEmptyEnv("LLM_MODEL"); v != "" {
		c.config.LLMService.Model = v
	}
	if v := firstNonEmptyEnv("LLM_TEMPERATURE"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 32); err == nil {
			c.config.LLMService.Temp = float32(parsed)
		}
	}
}

func (c *ConfigLoader) GetConfig() *Config {
	return c.config
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		v := strings.TrimSpace(os.Getenv(key))
		if v != "" {
			return v
		}
	}
	return ""
}

func resolveExistingPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	if filepath.IsAbs(p) {
		if fileExists(p) {
			return p
		}
		return ""
	}

	candidates := []string{
		p,
		filepath.Join("..", p),
		filepath.Join("..", "..", p),
		filepath.Join("..", "..", "..", p),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			abs, err := filepath.Abs(candidate)
			if err == nil {
				return abs
			}
			return candidate
		}
	}
	return ""
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
