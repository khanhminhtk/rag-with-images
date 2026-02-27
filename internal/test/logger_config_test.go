package test

import (
	"os"
	"path/filepath"
	"rag_imagetotext_texttoimage/internal/util"
	"testing"
)

func TestNewConfigLoader(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	if loader == nil {
		t.Fatal("NewConfigLoader() returned nil")
	}
}

func TestConfigLoader_Load_Success(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config == nil {
		t.Fatal("Config is nil")
	}

	if config.Qdrant.Bootstrap == "" {
		t.Error("Qdrant bootstrap is empty")
	}
	if config.Qdrant.Port == "" {
		t.Error("Qdrant port is empty")
	}

	if config.LLM.Model == "" {
		t.Error("LLM model is empty")
	}
	if config.LLM.Temp <= 0 {
		t.Error("LLM temp should be greater than 0")
	}

	t.Logf("Config loaded successfully:")
	t.Logf("  Qdrant: Bootstrap=%s, Port=%s, UseGRPC=%v",
		config.Qdrant.Bootstrap, config.Qdrant.Port, config.Qdrant.UseGRPC)
	t.Logf("  LLM: Model=%s, Temp=%.2f, ApiKey=%s",
		config.LLM.Model, config.LLM.Temp, config.LLM.ApiKey)
}

func TestConfigLoader_Load_YAMLFileNotFound(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/nonexistent.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	_, err := loader.Load()
	if err == nil {
		t.Error("Expected error for non-existent YAML file, got nil")
	}
	t.Logf("Got expected error: %v", err)
}

func TestConfigLoader_Load_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	tmpEnv := filepath.Join(tmpDir, "test.env")
	tmpYAML := filepath.Join(tmpDir, "invalid.yaml")

	err := os.WriteFile(tmpEnv, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp env file: %v", err)
	}

	err = os.WriteFile(tmpYAML, []byte("invalid: yaml: content: [[["), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp yaml file: %v", err)
	}

	loader := util.NewConfigLoader(tmpEnv, tmpYAML)
	_, err = loader.Load()
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
	t.Logf("Got expected error: %v", err)
}

func TestConfig_QdrantFields(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	tests := []struct {
		name     string
		field    string
		value    interface{}
		notEmpty bool
	}{
		{"Bootstrap", "Bootstrap", config.Qdrant.Bootstrap, true},
		{"Port", "Port", config.Qdrant.Port, true},
		{"UseGRPC", "UseGRPC", config.Qdrant.UseGRPC, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.notEmpty {
				if str, ok := tt.value.(string); ok && str == "" {
					t.Errorf("Field %s should not be empty", tt.field)
				}
			}
		})
	}
}

func TestConfig_LLMFields(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	tests := []struct {
		name  string
		field string
		check func() bool
	}{
		{
			name:  "Model",
			field: "Model",
			check: func() bool { return config.LLM.Model != "" },
		},
		{
			name:  "Temp",
			field: "Temp",
			check: func() bool { return config.LLM.Temp > 0 && config.LLM.Temp <= 2.0 },
		},
		{
			name:  "ApiKey",
			field: "ApiKey",
			check: func() bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check() {
				t.Errorf("Field %s validation failed", tt.field)
			}
		})
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	expectedBootstrap := "localhost"
	if config.Qdrant.Bootstrap != expectedBootstrap {
		t.Errorf("Expected bootstrap %s, got %s", expectedBootstrap, config.Qdrant.Bootstrap)
	}

	expectedPort := "6333"
	if config.Qdrant.Port != expectedPort {
		t.Errorf("Expected port %s, got %s", expectedPort, config.Qdrant.Port)
	}

	if config.Qdrant.UseGRPC != false {
		t.Errorf("Expected UseGRPC false, got %v", config.Qdrant.UseGRPC)
	}

	expectedModel := "gemini-3-flash-preview"
	if config.LLM.Model != expectedModel {
		t.Errorf("Expected model %s, got %s", expectedModel, config.LLM.Model)
	}

	expectedTemp := float32(0.7)
	if config.LLM.Temp != expectedTemp {
		t.Errorf("Expected temp %.2f, got %.2f", expectedTemp, config.LLM.Temp)
	}
}

func TestConfigLoader_ValidStructure(t *testing.T) {
	tmpDir := t.TempDir()
	tmpEnv := filepath.Join(tmpDir, "test.env")
	tmpYAML := filepath.Join(tmpDir, "valid.yaml")

	envContent := `TEST_API_KEY=test_key_12345`
	err := os.WriteFile(tmpEnv, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp env file: %v", err)
	}

	validYAML := `qdrant:
  bootstrap: "test-host"
  port: "9999"
  use_gRPC: true
llm:
  model: "test-model"
  temp: 1.5
  apikey: "${TEST_API_KEY}"
`
	err = os.WriteFile(tmpYAML, []byte(validYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp yaml file: %v", err)
	}

	loader := util.NewConfigLoader(tmpEnv, tmpYAML)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.Qdrant.Bootstrap != "test-host" {
		t.Errorf("Expected bootstrap 'test-host', got %s", config.Qdrant.Bootstrap)
	}
	if config.Qdrant.Port != "9999" {
		t.Errorf("Expected port '9999', got %s", config.Qdrant.Port)
	}
	if config.Qdrant.UseGRPC != true {
		t.Errorf("Expected UseGRPC true, got %v", config.Qdrant.UseGRPC)
	}

	if config.LLM.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got %s", config.LLM.Model)
	}
	if config.LLM.Temp != 1.5 {
		t.Errorf("Expected temp 1.5, got %.2f", config.LLM.Temp)
	}
	if config.LLM.ApiKey != "test_key_12345" {
		t.Errorf("Expected apikey 'test_key_12345', got %s", config.LLM.ApiKey)
	}
}

func TestConfigLoader_EnvVariableExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	tmpEnv := filepath.Join(tmpDir, "test.env")
	tmpYAML := filepath.Join(tmpDir, "test.yaml")

	testKey := "MY_TEST_API_KEY_12345"
	envContent := `MY_TEST_API_KEY=` + testKey
	err := os.WriteFile(tmpEnv, []byte(envContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp env file: %v", err)
	}

	yamlContent := `qdrant:
  bootstrap: "localhost"
  port: "6333"
  use_gRPC: false
llm:
  model: "test-model"
  temp: 0.8
  apikey: "${MY_TEST_API_KEY}"
`
	err = os.WriteFile(tmpYAML, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp yaml file: %v", err)
	}

	loader := util.NewConfigLoader(tmpEnv, tmpYAML)
	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if config.LLM.ApiKey != testKey {
		t.Errorf("Expected apikey '%s', got '%s'", testKey, config.LLM.ApiKey)
	}
	t.Logf("Environment variable correctly expanded: %s", config.LLM.ApiKey)
}

func TestConfigLoader_GetConfig(t *testing.T) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	loader := util.NewConfigLoader(envPath, yamlPath)
	_, err := loader.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	config := loader.GetConfig()
	if config == nil {
		t.Error("GetConfig() returned nil")
	}

	if config.Qdrant.Bootstrap == "" {
		t.Error("Bootstrap should not be empty")
	}
}

func BenchmarkConfigLoader_Load(b *testing.B) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	for i := 0; i < b.N; i++ {
		loader := util.NewConfigLoader(envPath, yamlPath)
		_, err := loader.Load()
		if err != nil {
			b.Fatalf("Failed to load config: %v", err)
		}
	}
}

func BenchmarkConfigLoader_LoadWithGetConfig(b *testing.B) {
	envPath := "../../config/.env"
	yamlPath := "../../config/config.yaml"

	for i := 0; i < b.N; i++ {
		loader := util.NewConfigLoader(envPath, yamlPath)
		_, err := loader.Load()
		if err != nil {
			b.Fatalf("Failed to load config: %v", err)
		}
		config := loader.GetConfig()
		if config == nil {
			b.Fatal("GetConfig() returned nil")
		}
	}
}
