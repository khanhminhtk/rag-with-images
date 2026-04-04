package kafka

import (
	"context"
	"fmt"
	"time"
	"errors"

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
	writer *segmentio.Writer
	appLogger  util.Logger
}

var _ ports.KafkaPublisher = (*Publisher)(nil)

func NewPublisher(client *KafkaClient, config PublisherConfig, appLogger util.Logger) (*Publisher, error) {
	if client == nil {
		appLogger.Error("internal.infra.kafka.publisher.NewPublisher: ", errors.New("Kafka client is nil"))
		return nil, errors.New("Kafka client is nil")
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
		p.appLogger.Error("internal.infra.kafka.publisher.Publish: ", errors.New("kafka publisher is not initialized"))
		return errors.New("kafka publisher is not initialized")
	}
	if input.Topic == "" {
		p.appLogger.Error("internal.infra.kafka.publisher.Publish: ", errors.New("topic is required"))
		return errors.New("topic is required")
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
		p.appLogger.Error("internal.infra.kafka.publisher.Publish: ", fmt.Errorf("publish message: %w", err))
		return fmt.Errorf("publish message: %w", err)
	}
	return nil
}

func (p *Publisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
