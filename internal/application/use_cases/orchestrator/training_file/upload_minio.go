package trainingfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
)

func (uc *trainingFileUseCase) UploadToMinio(ctx context.Context, folderDownload string, urlDownload string) (bool, error) {
	if strings.TrimSpace(folderDownload) == "" {
		err := errors.New("folder_download is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadToMinio missing folder_download", err)
		return false, err
	}

	if strings.TrimSpace(urlDownload) == "" {
		err := errors.New("url_download is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadToMinio missing url_download", err)
		return false, err
	}

	folderInfo, err := os.Stat(folderDownload)
	if err != nil {
		uc.logger.Error(
			"internal.application.use_cases.orchestrator.training_file.UploadToMinio invalid folder_download",
			err,
			"folder_download", folderDownload,
		)
		return false, fmt.Errorf("stat folder_download: %w", err)
	}
	if !folderInfo.IsDir() {
		err := fmt.Errorf("folder_download must be a directory")
		uc.logger.Error(
			"internal.application.use_cases.orchestrator.training_file.UploadToMinio invalid folder type",
			err,
			"folder_download", folderDownload,
		)
		return false, err
	}

	topic := strings.TrimSpace(uc.Config.MinIOService.Topics.UploadRequest)
	if topic == "" {
		err := errors.New("minio upload_request topic is empty")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadToMinio missing topics", err)
		return false, err
	}

	payload, err := json.Marshal(
		map[string]any{
			"folder_download": folderDownload,
			"url_download":   urlDownload,
		},
	)
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.UploadToMinio marshal payload failed", err)
		return false, fmt.Errorf("marshal payload: %w", err)
	}

	err = uc.KafkaPublisher.Publish(
		ctx,
		ports.PublishMessageInput{
			Topic: topic,
			Message: ports.KafkaMessage{
				Key:   []byte(fmt.Sprintf("upload-%s", filepath.Base(folderDownload))),
				Value: payload,
				Headers: map[string]string{
					"Content-Type":   "application/json",
					"Folder-Download": folderDownload,
					"URL-Download":    urlDownload,
					"source":          "training_file",
					"requested_at":    time.Now().UTC().Format(time.RFC3339),
				},
			},
		},
	)
	if err != nil {
		uc.logger.Error(
			"internal.application.use_cases.orchestrator.training_file.UploadToMinio publish failed",
			err,
			"topic", topic,
			"folder_download", folderDownload,
			"url_download", urlDownload,
		)
		return false, fmt.Errorf("publish minio upload request: %w", err)
	}

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.UploadToMinio publish succeeded",
		"topic", topic,
		"folder_download", folderDownload,
		"url_download", urlDownload,
	)

	return true, nil
}
