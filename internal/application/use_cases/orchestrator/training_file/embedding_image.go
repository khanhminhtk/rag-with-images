package trainingfile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
)

func (uc *trainingFileUseCase) EmbeddingBatchImageTraining(ctx context.Context, req *dtos.TrainingEmbeddingBatchImageRequest) (dtos.TrainingEmbeddingBatchImageResult, error) {
	startedAt := time.Now()
	ack := dtos.TrainingEmbeddingBatchImageResult{
		Vectors:   [][]float32{},
		Dimension: 0,
	}

	if req == nil {
		err := errors.New("request is nil")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining invalid request", err)
		return ack, err
	}

	if len(req.ImagePaths) == 0 {
		err := errors.New("image_paths is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining missing image_paths", err)
		return ack, err
	}

	topic := strings.TrimSpace(uc.Config.EmbeddingService.Topics.BatchImageRequest)
	if topic == "" {
		err := errors.New("embedding batch image request topic is empty")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining missing topic", err)
		return ack, err
	}

	sanitizedPaths := make([]string, 0, len(req.ImagePaths))
	for i, path := range req.ImagePaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			err := fmt.Errorf("image path at index %d is empty", i)
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining invalid image path", err, "index", i)
			return ack, err
		}
		sanitizedPaths = append(sanitizedPaths, trimmed)
	}

	images := make([][]byte, 0, len(sanitizedPaths))
	width := 0
	height := 0
	channels := 3

	for i, path := range sanitizedPaths {
		fileBytes, err := os.ReadFile(path)
		if err != nil {
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining read image failed", err, "path", path)
			return ack, fmt.Errorf("read image at index %d: %w", i, err)
		}

		img, _, err := image.Decode(bytes.NewReader(fileBytes))
		if err != nil {
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining decode image failed", err, "path", path)
			return ack, fmt.Errorf("decode image at index %d: %w", i, err)
		}

		rgbBytes, imgWidth, imgHeight := imageToRGBBytes(img)
		if i == 0 {
			width = imgWidth
			height = imgHeight
		} else if imgWidth != width || imgHeight != height {
			err := fmt.Errorf(
				"all images must have the same size: first=%dx%d current(index=%d)=%dx%d",
				width, height, i, imgWidth, imgHeight,
			)
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining inconsistent image size", err)
			return ack, err
		}

		images = append(images, rgbBytes)
	}

	payload, err := json.Marshal(map[string]any{
		"images":   images,
		"width":    width,
		"height":   height,
		"channels": channels,
	})
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining marshal payload failed", err)
		return ack, fmt.Errorf("marshal embedding batch image payload: %w", err)
	}

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining publish started",
		"topic", topic,
		"image_count", len(images),
		"width", width,
		"height", height,
		"channels", channels,
	)

	err = uc.KafkaPublisher.Publish(ctx, ports.PublishMessageInput{
		Topic: topic,
		Message: ports.KafkaMessage{
			Key:   []byte{},
			Value: payload,
			Headers: map[string]string{
				"source":       "training_file",
				"requested_at": time.Now().UTC().Format(time.RFC3339),
			},
		},
	})
	if err != nil {
		uc.logger.Error(
			"internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining publish failed",
			err,
			"topic", topic,
			"image_count", len(images),
		)
		return ack, fmt.Errorf("publish embedding batch image request: %w", err)
	}

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.EmbeddingBatchImageTraining publish succeeded",
		"topic", topic,
		"image_count", len(images),
		"latency_ms", time.Since(startedAt).Milliseconds(),
	)

	return ack, nil
}

func imageToRGBBytes(img image.Image) ([]byte, int, int) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	rgb := make([]byte, 0, width*height*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rgb = append(rgb, byte(r>>8), byte(g>>8), byte(b>>8))
		}
	}

	return rgb, width, height
}
