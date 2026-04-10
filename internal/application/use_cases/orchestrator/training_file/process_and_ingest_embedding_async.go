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

	"rag_imagetotext_texttoimage/internal/application/ports"
)

func (uc *trainingFileUseCase) embedTextAsyncByKafka(ctx context.Context, uuid string, texts []string, reqBatchSize int) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, errors.New("texts is empty")
	}
	if uc.KafkaPublisher == nil || uc.KafkaConsumer == nil {
		return nil, errors.New("kafka publisher/consumer is not configured")
	}

	topicReq := strings.TrimSpace(uc.Config.EmbeddingService.Topics.BatchTextRequest)
	topicRes := strings.TrimSpace(uc.Config.EmbeddingService.Topics.BatchTextResult)
	if topicReq == "" || topicRes == "" {
		return nil, errors.New("embedding text request/result topic is empty")
	}

	batchSize := uc.resolveTrainingBatchSize(reqBatchSize)

	groupID := "training-file-embed-text-" + uuid
	allEmbeddings := make([][]float32, 0, len(texts))

	for start, batchIndex := 0, 0; start < len(texts); start, batchIndex = start+batchSize, batchIndex+1 {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchTexts := texts[start:end]
		correlationID := fmt.Sprintf("embed-text-%s-%d-%d", uuid, batchIndex, time.Now().UnixNano())

		payload, err := json.Marshal(map[string]any{
			"correlation_id": correlationID,
			"texts":          batchTexts,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal embedding request for batch %d: %w", batchIndex, err)
		}

		if err := uc.KafkaPublisher.Publish(ctx, ports.PublishMessageInput{
			Topic: topicReq,
			Message: ports.KafkaMessage{
				Key:   []byte(correlationID),
				Value: payload,
				Headers: map[string]string{
					"correlation_id": correlationID,
					"source":         "training_file_orchestrator",
				},
			},
		}); err != nil {
			return nil, fmt.Errorf("publish embedding text request for batch %d: %w", batchIndex, err)
		}

		res, err := uc.pollEmbeddingResult(ctx, topicRes, groupID, correlationID)
		if err != nil {
			return nil, fmt.Errorf("poll embedding text result for batch %d: %w", batchIndex, err)
		}
		if strings.EqualFold(strings.TrimSpace(res.Status), "failed") {
			return nil, fmt.Errorf("embedding text failed for batch %d: %s", batchIndex, res.Message)
		}
		if len(res.Embeddings) != len(batchTexts) {
			return nil, fmt.Errorf("embedding result size mismatch for batch %d: expected=%d got=%d", batchIndex, len(batchTexts), len(res.Embeddings))
		}

		allEmbeddings = append(allEmbeddings, res.Embeddings...)
		uc.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.embedTextAsyncByKafka batch completed",
			"uuid", uuid,
			"batch_index", batchIndex,
			"batch_size", len(batchTexts),
			"processed", len(allEmbeddings),
			"total", len(texts),
		)
	}

	return allEmbeddings, nil
}

func (uc *trainingFileUseCase) embedSingleImageAsyncByKafka(ctx context.Context, uuid, imagePath string) ([]float32, error) {
	if uc.KafkaPublisher == nil || uc.KafkaConsumer == nil {
		return nil, errors.New("kafka publisher/consumer is not configured")
	}

	fileBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, err
	}
	rgbBytes, width, height := imageToRGBBytes(img)

	topicReq := strings.TrimSpace(uc.Config.EmbeddingService.Topics.BatchImageRequest)
	topicRes := strings.TrimSpace(uc.Config.EmbeddingService.Topics.BatchImageResult)
	if topicReq == "" || topicRes == "" {
		return nil, errors.New("embedding image request/result topic is empty")
	}

	correlationID := fmt.Sprintf("embed-image-%s-%d", uuid, time.Now().UnixNano())
	payload, err := json.Marshal(map[string]any{
		"correlation_id": correlationID,
		"images":         [][]byte{rgbBytes},
		"width":          width,
		"height":         height,
		"channels":       3,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding image request: %w", err)
	}

	if err := uc.KafkaPublisher.Publish(ctx, ports.PublishMessageInput{
		Topic: topicReq,
		Message: ports.KafkaMessage{
			Key:   []byte(correlationID),
			Value: payload,
			Headers: map[string]string{
				"correlation_id": correlationID,
				"source":         "training_file_orchestrator",
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("publish embedding image request: %w", err)
	}

	res, err := uc.pollEmbeddingResult(ctx, topicRes, "training-file-embed-image-"+uuid, correlationID)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(res.Status), "failed") {
		return nil, fmt.Errorf("embedding image failed: %s", res.Message)
	}
	if len(res.Embeddings) == 0 {
		return nil, errors.New("embedding image result is empty")
	}
	return res.Embeddings[0], nil
}

func (uc *trainingFileUseCase) pollEmbeddingResult(ctx context.Context, topic, groupID, correlationID string) (embeddingBatchResult, error) {
	result := embeddingBatchResult{}
	timeoutCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	foundCh := make(chan embeddingBatchResult, 1)
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		consumeErr := uc.KafkaConsumer.Consume(timeoutCtx, topic, groupID, func(hctx context.Context, msg ports.ConsumeMessage) error {
			var payload embeddingBatchResult
			if err := json.Unmarshal(msg.Message.Value, &payload); err != nil {
				return nil
			}

			cid := strings.TrimSpace(payload.CorrelationID)
			if cid == "" {
				cid = strings.TrimSpace(string(msg.Message.Key))
			}
			if cid == "" {
				cid = strings.TrimSpace(msg.Message.Headers["correlation_id"])
			}
			if cid != correlationID {
				return nil
			}

			payload.CorrelationID = cid
			select {
			case foundCh <- payload:
			default:
			}
			cancel()
			return nil
		})
		if consumeErr != nil && !errors.Is(consumeErr, context.Canceled) && !errors.Is(consumeErr, context.DeadlineExceeded) {
			errCh <- consumeErr
		}
	}()

	select {
	case res := <-foundCh:
		result = res
		<-doneCh
		return result, nil
	case err := <-errCh:
		<-doneCh
		return result, err
	case <-timeoutCtx.Done():
		<-doneCh
		return result, fmt.Errorf("poll embedding result timeout for correlation_id=%s", correlationID)
	}
}
