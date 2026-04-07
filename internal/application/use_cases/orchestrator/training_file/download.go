package trainingfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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

	resp, err := http.Get(req.UrlDownFile)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to download file", err, "url_down_file", req.UrlDownFile)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to download file", err, "url_down_file", req.UrlDownFile)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, err
	}

	open_file, err := os.Create(savePath)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to create file", err, "save_path", savePath)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to create file: %w", err)
	}
	defer open_file.Close()

	_, err = io.Copy(open_file, resp.Body)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.Download failed to save file", err, "save_path", savePath)
		return dtos.DownFileTrainingResult{Success: false, FilePath: ""}, fmt.Errorf("internal.application.use_cases.orchestrator.training_file.Download failed to save file: %w", err)
	}

	uc.logger.Info("internal.application.use_cases.orchestrator.training_file.Download file downloaded successfully", "url_down_file", req.UrlDownFile, "save_path", savePath)
	return dtos.DownFileTrainingResult{Success: true, FilePath: savePath}, nil
}

func getFileNameFromURL(url string) string {
	_, file := filepath.Split(url)
	return file
}
