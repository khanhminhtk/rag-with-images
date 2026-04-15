package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"rag_imagetotext_texttoimage/internal/util"
)

type dlmodelJinaRuntimeConfig struct {
	path    string
	cleanup func()
}

type jinaRuntimeConfig struct {
	Models struct {
		JinaTextEncoder struct {
			ModelPath string `yaml:"model_path"`
			VocabPath string `yaml:"vocab_path"`
		} `yaml:"jina_text_encoder"`
		JinaVisionEncoder struct {
			ModelPath string `yaml:"model_path"`
		} `yaml:"jina_vision_encoder"`
	} `yaml:"models"`
}

func prepareJinaRuntime(rawConfigPath string) (string, func(), error) {
	absConfigPath, err := filepath.Abs(strings.TrimSpace(rawConfigPath))
	if err != nil {
		return "", func() {}, err
	}
	configDir := filepath.Dir(absConfigPath)
	runtimeRoot := filepath.Clean(filepath.Join(configDir, ".."))
	if _, err := os.Stat(filepath.Join(runtimeRoot, "model")); err != nil {
		runtimeRoot = configDir
	}

	raw, err := os.ReadFile(absConfigPath)
	if err != nil {
		return "", func() {}, err
	}

	cfg := map[interface{}]interface{}{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return "", func() {}, err
	}

	if p, ok := getNestedString(cfg, "models", "jina_text_encoder", "model_path"); ok {
		_ = setNestedString(cfg, resolveMaybeRelative(runtimeRoot, p), "models", "jina_text_encoder", "model_path")
	}
	if p, ok := getNestedString(cfg, "models", "jina_text_encoder", "vocab_path"); ok {
		_ = setNestedString(cfg, resolveMaybeRelative(runtimeRoot, p), "models", "jina_text_encoder", "vocab_path")
	}
	if p, ok := getNestedString(cfg, "models", "jina_vision_encoder", "model_path"); ok {
		_ = setNestedString(cfg, resolveMaybeRelative(runtimeRoot, p), "models", "jina_vision_encoder", "model_path")
	}

	normalized, err := yaml.Marshal(cfg)
	if err != nil {
		return "", func() {}, err
	}

	tmpFile, err := os.CreateTemp("", "jina-runtime-config-*.yaml")
	if err != nil {
		return "", func() {}, err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(normalized); err != nil {
		return "", func() {}, err
	}

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanup, nil
}

func resolveJinaConfigPath(rawConfigPath string) (string, error) {
	p := strings.TrimSpace(rawConfigPath)
	if p == "" {
		return "", fmt.Errorf("jina config path is empty")
	}

	if filepath.IsAbs(p) {
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}

	candidates := make([]string, 0, 4)
	candidates = append(candidates, p)

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, p))
	}

	if _, currentFile, _, ok := runtime.Caller(0); ok {
		bootstrapDir := filepath.Dir(currentFile)
		candidates = append(candidates, filepath.Join(bootstrapDir, p))
		repoRoot := filepath.Clean(filepath.Join(bootstrapDir, "..", ".."))
		candidates = append(candidates, filepath.Join(repoRoot, p))
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, statErr := os.Stat(abs)
		if statErr == nil && !info.IsDir() {
			return abs, nil
		}
	}

	return "", fmt.Errorf("cannot resolve jina config path from %q", rawConfigPath)
}

func resolveMaybeRelative(baseDir string, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(baseDir, p))
}

func getNestedString(root map[interface{}]interface{}, keys ...string) (string, bool) {
	var current interface{} = root
	for _, key := range keys {
		m, ok := current.(map[interface{}]interface{})
		if !ok {
			return "", false
		}
		current, ok = m[key]
		if !ok {
			return "", false
		}
	}
	value, ok := current.(string)
	if !ok {
		return "", false
	}
	return value, true
}

func setNestedString(root map[interface{}]interface{}, value string, keys ...string) bool {
	if len(keys) == 0 {
		return false
	}
	var current interface{} = root
	for i := 0; i < len(keys)-1; i++ {
		m, ok := current.(map[interface{}]interface{})
		if !ok {
			return false
		}
		next, ok := m[keys[i]]
		if !ok {
			return false
		}
		current = next
	}
	m, ok := current.(map[interface{}]interface{})
	if !ok {
		return false
	}
	m[keys[len(keys)-1]] = value
	return true
}

func validateJinaRuntimeConfig(configPath string) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read runtime config: %w", err)
	}

	cfg := jinaRuntimeConfig{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("parse runtime config: %w", err)
	}

	required := map[string]string{
		"jina_text_model_path":   cfg.Models.JinaTextEncoder.ModelPath,
		"jina_text_vocab_path":   cfg.Models.JinaTextEncoder.VocabPath,
		"jina_vision_model_path": cfg.Models.JinaVisionEncoder.ModelPath,
	}
	for key, p := range required {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("%s is empty in runtime config", key)
		}
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("%s not found at %s (hint: build/export third_party/onnx_c++ assets)", key, p)
		}
	}

	candidateRoots := resolveOnnxRuntimeRoots(required)
	foundBuildArtifacts := false
	for _, root := range candidateRoots {
		if hasEdgeSentinelBuildArtifacts(root) {
			foundBuildArtifacts = true
			break
		}
	}
	if !foundBuildArtifacts {
		return fmt.Errorf(
			"missing edge_sentinel build artifacts from runtime paths (searched roots=%v; hint: run cmake -S third_party/onnx_c++ -B build-debug && cmake --build build-debug)",
			candidateRoots,
		)
	}

	return nil
}

func resolveOnnxRuntimeRoots(required map[string]string) []string {
	roots := make([]string, 0)
	seen := make(map[string]struct{})
	for _, p := range required {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		current := filepath.Dir(abs)
		for {
			if filepath.Base(current) == "onnx_c++" {
				if _, ok := seen[current]; !ok {
					seen[current] = struct{}{}
					roots = append(roots, current)
				}
			}
			candidate := filepath.Join(current, "third_party", "onnx_c++")
			if _, err := os.Stat(candidate); err == nil {
				if _, ok := seen[candidate]; !ok {
					seen[candidate] = struct{}{}
					roots = append(roots, candidate)
				}
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return roots
}

func hasEdgeSentinelBuildArtifacts(onnxRoot string) bool {
	patterns := []string{
		filepath.Join(onnxRoot, "build", "src", "libedge_sentinel_lib.*"),
		filepath.Join(onnxRoot, "build-debug", "src", "libedge_sentinel_lib.*"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

func waitConsumerStopped(errCh <-chan error, consumerName string, logger util.Logger, timeout time.Duration) {
	if errCh == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case err, ok := <-errCh:
			if !ok {
				logger.Info("consumer stopped", "consumer", consumerName)
				return
			}
			if err != nil {
				logger.Error("consumer stopped with error", err, "consumer", consumerName)
			}
		case <-timer.C:
			logger.Error("consumer stop timeout", fmt.Errorf("timeout waiting for shutdown"), "consumer", consumerName)
			return
		}
	}
}
