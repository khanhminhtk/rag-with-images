package trainingfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (uc *trainingFileUseCase) ReadMarkdownFile(ctx context.Context, markdownPath string) (string, error) {
	startedAt := time.Now()

	if err := ctx.Err(); err != nil {
		if uc.logger != nil {
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile context canceled", err)
		}
		return "", err
	}

	path := strings.TrimSpace(markdownPath)
	if path == "" {
		err := errors.New("markdown_path is required")
		if uc.logger != nil {
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile missing markdown_path", err)
		}
		return "", err
	}

	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(path)))
	if ext != ".md" && ext != ".markdown" {
		err := fmt.Errorf("unsupported markdown extension: %s", ext)
		if uc.logger != nil {
			uc.logger.Error(
				"internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile invalid extension",
				err,
				"markdown_path", path,
			)
		}
		return "", err
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if uc.logger != nil {
			uc.logger.Error(
				"internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile stat file failed",
				err,
				"markdown_path", path,
			)
		}
		return "", fmt.Errorf("stat markdown file: %w", err)
	}
	if fileInfo.IsDir() {
		err := errors.New("markdown_path must be a file")
		if uc.logger != nil {
			uc.logger.Error(
				"internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile path is directory",
				err,
				"markdown_path", path,
			)
		}
		return "", err
	}

	contentBytes, err := os.ReadFile(path)
	if err != nil {
		if uc.logger != nil {
			uc.logger.Error(
				"internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile read file failed",
				err,
				"markdown_path", path,
			)
		}
		return "", fmt.Errorf("read markdown file: %w", err)
	}

	content := strings.TrimSpace(string(contentBytes))
	if content == "" {
		err := errors.New("markdown file is empty")
		if uc.logger != nil {
			uc.logger.Error(
				"internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile empty markdown content",
				err,
				"markdown_path", path,
			)
		}
		return "", err
	}

	if uc.logger != nil {
		uc.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.ReadMarkdownFile succeeded",
			"markdown_path", path,
			"content_size_bytes", len(contentBytes),
			"latency_ms", time.Since(startedAt).Milliseconds(),
		)
	}

	return content, nil
}
