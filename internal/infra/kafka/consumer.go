package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	segmentio "github.com/segmentio/kafka-go"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type ConsumerConfig struct {
	MinBytes       int
	MaxBytes       int
	CommitInterval time.Duration
	StartOffset    int64
}

type Consumer struct {
	client    *KafkaClient
	config    ConsumerConfig
	appLogger util.Logger
	mu        sync.Mutex
	readers   []*segmentio.Reader
}

var _ ports.KafkaConsumer = (*Consumer)(nil)

func NewKafkaConsumer(client *KafkaClient, config ConsumerConfig, appLogger util.Logger) (*Consumer, error) {
	if client == nil {
		err := errors.New("kafka client is nil")
		if appLogger != nil {
			appLogger.Error("new consumer failed", err)
		}
		return nil, err
	}
	if config.MinBytes <= 0 {
		config.MinBytes = 1
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = 10e6
	}
	if appLogger != nil {
		appLogger.Info(
			"kafka consumer initialized",
			"min_bytes", config.MinBytes,
			"max_bytes", config.MaxBytes,
			"commit_interval", config.CommitInterval.String(),
			"start_offset", config.StartOffset,
		)
	}
	return &Consumer{
		client:    client,
		config:    config,
		appLogger: appLogger,
	}, nil
}

func (c *Consumer) Consume(ctx context.Context, topic string, groupID string, handler ports.MessageHandler) error {
	if c == nil || c.client == nil {
		err := errors.New("kafka client is nil")
		if c != nil && c.appLogger != nil {
			c.appLogger.Error("consume failed", err, "topic", topic, "group_id", groupID)
		}
		return err
	}
	if topic == "" {
		err := errors.New("topic is required")
		if c.appLogger != nil {
			c.appLogger.Error("consume failed", err, "group_id", groupID)
		}
		return err
	}
	if groupID == "" {
		err := errors.New("group_id is required")
		if c.appLogger != nil {
			c.appLogger.Error("consume failed", err, "topic", topic)
		}
		return err
	}
	if handler == nil {
		err := errors.New("handler is required")
		if c.appLogger != nil {
			c.appLogger.Error("consume failed", err, "topic", topic, "group_id", groupID)
		}
		return err
	}

	readerConfig := segmentio.ReaderConfig{
		Brokers:  c.client.Brokers(),
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: c.config.MinBytes,
		MaxBytes: c.config.MaxBytes,
		Dialer:   c.client.Dialer(),
	}
	if c.config.CommitInterval > 0 {
		readerConfig.CommitInterval = c.config.CommitInterval
	}
	if c.config.StartOffset != 0 {
		readerConfig.StartOffset = c.config.StartOffset
	}

	reader := segmentio.NewReader(readerConfig)
	c.addReader(reader)
	defer c.removeReader(reader)
	defer reader.Close()

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			wrappedErr := fmt.Errorf("fetch message: %w", err)
			if c.appLogger != nil {
				c.appLogger.Error("consume fetch failed", wrappedErr, "topic", topic, "group_id", groupID)
			}
			return wrappedErr
		}

		headers := make(map[string]string, len(msg.Headers))
		for _, header := range msg.Headers {
			headers[header.Key] = string(header.Value)
		}

		consumeMessage := ports.ConsumeMessage{
			Topic:     msg.Topic,
			Partition: msg.Partition,
			Offset:    msg.Offset,
			Message: ports.KafkaMessage{
				Key:     msg.Key,
				Value:   msg.Value,
				Headers: headers,
			},
		}

		if err := handler(ctx, consumeMessage); err != nil {
			wrappedErr := fmt.Errorf("consume handler failed: %w", err)
			if c.appLogger != nil {
				c.appLogger.Error("consume handler failed", wrappedErr, "topic", topic, "group_id", groupID, "offset", msg.Offset)
			}
			return wrappedErr
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			wrappedErr := fmt.Errorf("commit message: %w", err)
			if c.appLogger != nil {
				c.appLogger.Error("consume commit failed", wrappedErr, "topic", topic, "group_id", groupID, "offset", msg.Offset)
			}
			return wrappedErr
		}
	}
}

func (c *Consumer) Close() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	readers := make([]*segmentio.Reader, len(c.readers))
	copy(readers, c.readers)
	c.readers = nil
	c.mu.Unlock()

	var errs []error
	for _, reader := range readers {
		if reader == nil {
			continue
		}
		if err := reader.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Consumer) addReader(reader *segmentio.Reader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readers = append(c.readers, reader)
}

func (c *Consumer) removeReader(reader *segmentio.Reader) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, r := range c.readers {
		if r != reader {
			continue
		}
		c.readers = append(c.readers[:i], c.readers[i+1:]...)
		return
	}
}
