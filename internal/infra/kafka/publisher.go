package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	segmentio "github.com/segmentio/kafka-go"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type PublisherConfig struct {
	RequiredAcks int
	Balancer     segmentio.Balancer
	BatchBytes   int
	BatchTimeout time.Duration
}

type Publisher struct {
	writer    *segmentio.Writer
	appLogger util.Logger
}

var _ ports.KafkaPublisher = (*Publisher)(nil)

func NewPublisher(client *KafkaClient, config PublisherConfig, appLogger util.Logger) (*Publisher, error) {
	if client == nil {
		err := errors.New("kafka client is nil")
		if appLogger != nil {
			appLogger.Error("new publisher failed", err)
		}
		return nil, err
	}

	batchTimeout := config.BatchTimeout
	if batchTimeout <= 0 {
		batchTimeout = 100 * time.Millisecond
	}

	balancer := config.Balancer
	if balancer == nil {
		balancer = &segmentio.LeastBytes{}
	}

	writerConfig := segmentio.WriterConfig{
		Brokers:      client.Brokers(),
		Dialer:       client.Dialer(),
		Balancer:     balancer,
		RequiredAcks: config.RequiredAcks,
		BatchTimeout: batchTimeout,
	}
	if config.BatchBytes > 0 {
		writerConfig.BatchBytes = config.BatchBytes
	}
	writer := segmentio.NewWriter(writerConfig)

	return &Publisher{writer: writer, appLogger: appLogger}, nil
}

func (p *Publisher) Publish(ctx context.Context, input ports.PublishMessageInput) error {
	if p == nil || p.writer == nil {
		err := errors.New("kafka publisher is not initialized")
		if p != nil && p.appLogger != nil {
			p.appLogger.Error("publish failed", err, "topic", input.Topic)
		}
		return err
	}
	if input.Topic == "" {
		err := errors.New("topic is required")
		if p.appLogger != nil {
			p.appLogger.Error("publish failed", err)
		}
		return err
	}

	headers := make([]segmentio.Header, 0, len(input.Message.Headers))
	for key, value := range input.Message.Headers {
		headers = append(headers, segmentio.Header{
			Key:   key,
			Value: []byte(value),
		})
	}

	msg := segmentio.Message{
		Topic:   input.Topic,
		Key:     input.Message.Key,
		Value:   input.Message.Value,
		Headers: headers,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		wrappedErr := fmt.Errorf("publish message: %w", err)
		if p.appLogger != nil {
			p.appLogger.Error("publish message failed", wrappedErr, "topic", input.Topic)
		}
		return wrappedErr
	}
	return nil
}

func (p *Publisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
