package chat

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/util"
)

const imageDownloadTimeout = 30 * time.Second

func (c *ChatbotHandler) prepareImageForSession(ctx context.Context, imagePath string, sessionID string) (string, error) {
	trimmed := strings.TrimSpace(imagePath)
	if trimmed == "" {
		return "", nil
	}

	parsedURL, err := neturl.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid image_path: %w", err)
	}
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return trimmed, nil
	}

	sessionDir, err := sessionTmpDir(sessionID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", fmt.Errorf("create session temp dir: %w", err)
	}

	filename := downloadableImageName(parsedURL)
	savePath := filepath.Join(sessionDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), filename))
	startedAt := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmed, nil)
	if err != nil {
		return "", fmt.Errorf("build image download request: %w", err)
	}
	resp, err := (&http.Client{Timeout: imageDownloadTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image failed with status code %d", resp.StatusCode)
	}

	file, err := os.Create(savePath)
	if err != nil {
		return "", fmt.Errorf("create local image file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", fmt.Errorf("save downloaded image: %w", err)
	}

	if c != nil && c.appLogger != nil {
		c.appLogger.Info(
			"internal.application.use_cases.orchestrator.chat.prepareImageForSession image downloaded",
			"session_id", sessionID,
			"source_url", trimmed,
			"saved_path", savePath,
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	return savePath, nil
}

func sessionTmpDir(sessionID string) (string, error) {
	cleanSessionID := strings.TrimSpace(sessionID)
	if cleanSessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}
	if strings.Contains(cleanSessionID, "/") || strings.Contains(cleanSessionID, "\\") || cleanSessionID == "." || cleanSessionID == ".." {
		return "", fmt.Errorf("session_id contains invalid path characters")
	}
	return filepath.Join("data", "tmp", cleanSessionID), nil
}

func cleanupSessionTmpDir(sessionID string, logger util.Logger) {
	sessionDir, err := sessionTmpDir(sessionID)
	if err != nil {
		if logger != nil {
			logger.Error("internal.application.use_cases.orchestrator.chat.cleanupSessionTmpDir invalid session id", err, "session_id", sessionID)
		}
		return
	}
	if err := os.RemoveAll(sessionDir); err != nil {
		if logger != nil {
			logger.Error("internal.application.use_cases.orchestrator.chat.cleanupSessionTmpDir failed", err, "session_id", sessionID, "session_dir", sessionDir)
		}
		return
	}
	if logger != nil {
		logger.Info(
			"internal.application.use_cases.orchestrator.chat.cleanupSessionTmpDir completed",
			"session_id", sessionID,
			"session_dir", sessionDir,
		)
	}
}

func downloadableImageName(parsedURL *neturl.URL) string {
	if parsedURL == nil {
		return "downloaded_image"
	}
	fileName := filepath.Base(parsedURL.Path)
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || fileName == "." || fileName == "/" {
		return "downloaded_image"
	}
	return fileName
}
