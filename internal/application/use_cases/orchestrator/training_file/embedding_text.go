package trainingfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
)

func (uc *trainingFileUseCase) EmbeddingBatchTextTraining(ctx context.Context, req *dtos.TrainingEmbeddingBatchTextRequest) (dtos.TrainingEmbeddingBatchTextResult, error) {
	startedAt := time.Now()
	ack := dtos.TrainingEmbeddingBatchTextResult{
		Vectors:   [][]float32{},
		Dimension: 0,
	}

	if req == nil {
		err := errors.New("request is nil")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining invalid request", err)
		return ack, err
	}

	if len(req.Texts) == 0 {
		err := errors.New("texts is required")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining missing texts", err)
		return ack, err
	}

	sanitizedTexts := make([]string, 0, len(req.Texts))
	for i, text := range req.Texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			err := fmt.Errorf("text at index %d is empty", i)
			uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining invalid text item", err, "index", i)
			return ack, err
		}
		sanitizedTexts = append(sanitizedTexts, trimmed)
	}

	topic := strings.TrimSpace(uc.Config.EmbeddingService.Topics.BatchTextRequest)
	if topic == "" {
		err := errors.New("embedding batch text request topic is empty")
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining missing topic", err)
		return ack, err
	}

	payload, err := json.Marshal(map[string]any{
		"texts": sanitizedTexts,
	})
	if err != nil {
		uc.logger.Error("internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining marshal payload failed", err)
		return ack, fmt.Errorf("marshal embedding batch text payload: %w", err)
	}

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining publish started",
		"topic", topic,
		"text_count", len(sanitizedTexts),
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
			"internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining publish failed",
			err,
			"topic", topic,
			"text_count", len(sanitizedTexts),
		)
		return ack, fmt.Errorf("publish embedding batch text request: %w", err)
	}

	uc.logger.Info(
		"internal.application.use_cases.orchestrator.training_file.EmbeddingBatchTextTraining publish succeeded",
		"topic", topic,
		"text_count", len(sanitizedTexts),
		"latency_ms", time.Since(startedAt).Milliseconds(),
	)

	return ack, nil
}
