package test

import (
	"os"
	"path/filepath"
	"rag_imagetotext_texttoimage/internal/util"
	"testing"
)

func TestYAMLConfigLoader_Success(t *testing.T) {
	configLoader := util.NewYAMLConfig()

	config, err := configLoader.LoadConfig("../../config/config.yaml")
	if err != nil {
		t.Fatalf("Failed to load YAML config: %v", err)
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

	t.Logf("Config loaded: Bootstrap=%s, Port=%s, UseGRPC=%v",
		config.Qdrant.Bootstrap, config.Qdrant.Port, config.Qdrant.UseGRPC)
}

func TestYAMLConfigLoader_FileNotFound(t *testing.T) {
	configLoader := util.NewYAMLConfig()

	_, err := configLoader.LoadConfig("../../config/nonexistent.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestYAMLConfigLoader_InvalidYAML(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "invalid-config.yaml")
	err := os.WriteFile(tmpFile, []byte("invalid: yaml: content: ["), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	configLoader := util.NewYAMLConfig()
	_, err = configLoader.LoadConfig(tmpFile)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestYAMLConfigLoader_EmptyFile(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "empty-config.yaml")
	err := os.WriteFile(tmpFile, []byte(""), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	configLoader := util.NewYAMLConfig()
	_, err = configLoader.LoadConfig(tmpFile)

	if err == nil {
		t.Error("Expected error for empty file, got nil")
	}

	t.Logf("Empty file correctly returns error: %v", err)
}

func TestConfig_QdrantFields(t *testing.T) {
	configLoader := util.NewYAMLConfig()
	config, err := configLoader.LoadConfig("../../config/config.yaml")
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

func TestConfigLoader_Interface(t *testing.T) {
	var _ util.ConfigLoader = (*util.YAMLConfig)(nil)
	t.Log("YAMLConfig implements ConfigLoader interface")
}

func TestNewYAMLConfig(t *testing.T) {
	loader := util.NewYAMLConfig()
	if loader == nil {
		t.Error("NewYAMLConfig() returned nil")
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	configLoader := util.NewYAMLConfig()
	config, err := configLoader.LoadConfig("../../config/config.yaml")
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
}

func TestYAMLConfigLoader_ValidStructure(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "valid-config.yaml")
	validYAML := `qdrant:
  bootstrap: "test-host"
  port: "9999"
  use_gRPC: true
`
	err := os.WriteFile(tmpFile, []byte(validYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	configLoader := util.NewYAMLConfig()
	config, err := configLoader.LoadConfig(tmpFile)
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
}

func BenchmarkYAMLConfigLoader(b *testing.B) {
	configLoader := util.NewYAMLConfig()

	for i := 0; i < b.N; i++ {
		_, err := configLoader.LoadConfig("../../config/config.yaml")
		if err != nil {
			b.Fatalf("Failed to load config: %v", err)
		}
	}
}
