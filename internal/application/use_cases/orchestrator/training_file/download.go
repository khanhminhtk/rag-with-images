package trainingfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
)

func (uc *trainingFileUseCase) Download(ctx context.Context, req *dtos.DownFileTrainingRequest) (dtos.DownFileTrainingResult, error) {
	if req == nil {
		err := errors.New("request is nil")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download invalid request", err)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, err
	}

	if req.UrlDownFile == "" {
		err := errors.New("url_down_file is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download missing url_down_file", err)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, err
	}

	if req.Uuid == "" {
		err := errors.New("uuid is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download missing uuid", err)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, err
	}

	if req.PathSave == "" {
		err := errors.New("path_save is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download missing path_save", err)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, err
	}

	if err := os.MkdirAll(req.PathSave, 0755); err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to create directory", err, "path_save", req.PathSave)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to create directory: %w", err)
	}

	fileName := getFileNameFromURL(req.UrlDownFile)
	savePath := filepath.Join(req.PathSave, fmt.Sprintf("%s_%s", req.Uuid, fileName))

	open_file, err := os.Create(savePath)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to create file", err, "save_path", savePath)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to create file: %w", err)
	}
	defer open_file.Close()

	reader, err := uc.openDownloadReader(ctx, req.UrlDownFile)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to download file", err, "url_down_file", req.UrlDownFile)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to download file: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(open_file, reader)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to save file", err, "save_path", savePath)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to save file: %w", err)
	}

	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.Download file downloaded successfully", "url_down_file", req.UrlDownFile, "save_path", savePath)
	return dtos.DownFileTrainingResult{Success: true, FilePath: savePath}, nil
}

func (uc *trainingFileUseCase) openDownloadReader(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := neturl.Parse(trimmed)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, trimmed, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		return resp.Body, nil
	case "file":
		localPath := parsed.Path
		if localPath == "" {
			return nil, errors.New("file url path is empty")
		}
		return os.Open(localPath)
	case "":
		// Treat plain paths as local file paths.
		return os.Open(trimmed)
	default:
		return nil, fmt.Errorf("unsupported protocol scheme %q", parsed.Scheme)
	}
}

func getFileNameFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "downloaded_file"
	}

	parsed, err := neturl.Parse(trimmed)
	if err == nil {
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "file":
			_, file := filepath.Split(parsed.Path)
			if strings.TrimSpace(file) != "" {
				return file
			}
		}
	}

	_, file := filepath.Split(trimmed)
	if strings.TrimSpace(file) != "" {
		return file
	}
	return "downloaded_file"
}
