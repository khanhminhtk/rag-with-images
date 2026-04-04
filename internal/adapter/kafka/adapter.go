package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
)

type InfraAdapterConfig struct {
	Brokers []string

	DialTimeout time.Duration

	Publisher infraKafka.PublisherConfig
	Consumer  infraKafka.ConsumerConfig
}

type ProducerAdapter struct {
	publisher ports.KafkaPublisher
	logger    util.Logger
}

type ConsumerAdapter struct {
	consumer ports.KafkaConsumer
	logger   util.Logger
}

func NewInfraAdapters(cfg InfraAdapterConfig, appLogger util.Logger) (*ProducerAdapter, *ConsumerAdapter, error) {
	if appLogger != nil {
		appLogger.Info(
			"internal.adapter.kafka.NewInfraAdapters: initializing kafka adapters",
			"brokers", cfg.Brokers,
			"dial_timeout", cfg.DialTimeout.String(),
		)
	}

	client, err := infraKafka.NewKafkaClient(infraKafka.KafkaConfig{
		Brokers:     cfg.Brokers,
		DialTimeout: cfg.DialTimeout,
	}, appLogger)
	if err != nil {
		if appLogger != nil {
			appLogger.Error("internal.adapter.kafka.NewInfraAdapters: create kafka client failed", err, "brokers", cfg.Brokers)
		}
		return nil, nil, fmt.Errorf("create kafka client: %w", err)
	}
	if appLogger != nil {
		appLogger.Info("internal.adapter.kafka.NewInfraAdapters: kafka client created", "brokers", cfg.Brokers)
	}

	publisher, err := infraKafka.NewPublisher(client, cfg.Publisher, appLogger)
	if err != nil {
		if appLogger != nil {
			appLogger.Error("internal.adapter.kafka.NewInfraAdapters: create kafka publisher failed", err)
		}
		return nil, nil, fmt.Errorf("create kafka publisher: %w", err)
	}
	if appLogger != nil {
		appLogger.Info("internal.adapter.kafka.NewInfraAdapters: kafka publisher created")
	}

	consumer, err := infraKafka.NewKafkaConsumer(client, cfg.Consumer, appLogger)
	if err != nil {
		_ = publisher.Close()
		if appLogger != nil {
			appLogger.Error("internal.adapter.kafka.NewInfraAdapters: create kafka consumer failed", err)
		}
		return nil, nil, fmt.Errorf("create kafka consumer: %w", err)
	}
	if appLogger != nil {
		appLogger.Info("internal.adapter.kafka.NewInfraAdapters: kafka consumer created")
	}

	return &ProducerAdapter{
			publisher: publisher,
			logger:    appLogger,
		}, &ConsumerAdapter{
			consumer: consumer,
			logger:   appLogger,
		}, nil
}

func NewProducerAdapter(publisher ports.KafkaPublisher) *ProducerAdapter {
	return &ProducerAdapter{publisher: publisher}
}

func NewConsumerAdapter(consumer ports.KafkaConsumer, appLogger util.Logger) *ConsumerAdapter {
	return &ConsumerAdapter{
		consumer: consumer,
		logger:   appLogger,
	}
}

func (a *ProducerAdapter) Publish(ctx context.Context, topic string, key []byte, value []byte, headers map[string]string) error {
	if a == nil || a.publisher == nil {
		return fmt.Errorf("producer adapter is not initialized")
	}
	if a.logger != nil {
		a.logger.Info(
			"internal.adapter.kafka.ProducerAdapter.Publish: publishing message",
			"topic", topic,
			"key_len", len(key),
			"value_len", len(value),
			"headers_count", len(headers),
		)
	}

	err := a.publisher.Publish(ctx, ports.PublishMessageInput{
		Topic: topic,
		Message: ports.KafkaMessage{
			Key:     key,
			Value:   value,
			Headers: headers,
		},
	})
	if err != nil {
		if a.logger != nil {
			a.logger.Error("internal.adapter.kafka.ProducerAdapter.Publish: publish failed", err, "topic", topic)
		}
		return fmt.Errorf("publish to topic %s: %w", topic, err)
	}
	if a.logger != nil {
		a.logger.Info("internal.adapter.kafka.ProducerAdapter.Publish: publish succeeded", "topic", topic)
	}
	return nil
}

func (a *ProducerAdapter) PublishJSON(ctx context.Context, topic string, key []byte, payload any, headers map[string]string) error {
	value, err := json.Marshal(payload)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("internal.adapter.kafka.ProducerAdapter.PublishJSON: marshal payload failed", err, "topic", topic)
		}
		return fmt.Errorf("marshal payload: %w", err)
	}
	if a.logger != nil {
		a.logger.Debug("internal.adapter.kafka.ProducerAdapter.PublishJSON: payload marshaled", "topic", topic, "value_len", len(value))
	}
	return a.Publish(ctx, topic, key, value, headers)
}

func (a *ProducerAdapter) Close() error {
	if a == nil || a.publisher == nil {
		return nil
	}
	err := a.publisher.Close()
	if err != nil {
		if a.logger != nil {
			a.logger.Error("internal.adapter.kafka.ProducerAdapter.Close: close publisher failed", err)
		}
		return err
	}
	if a.logger != nil {
		a.logger.Info("internal.adapter.kafka.ProducerAdapter.Close: publisher closed")
	}
	return nil
}

func (a *ConsumerAdapter) Start(ctx context.Context, topic string, groupID string, handler ports.MessageHandler) <-chan error {
	errCh := make(chan error, 1)
	if a == nil || a.consumer == nil {
		errCh <- fmt.Errorf("consumer adapter is not initialized")
		close(errCh)
		return errCh
	}
	if a.logger != nil {
		a.logger.Info(
			"internal.adapter.kafka.ConsumerAdapter.Start: starting consumer loop",
			"topic", topic,
			"group_id", groupID,
		)
	}

	go func() {
		defer close(errCh)
		err := a.consumer.Consume(ctx, topic, groupID, handler)
		if err != nil {
			if a.logger != nil {
				a.logger.Error("kafka consumer stopped", err, "topic", topic, "group_id", groupID)
			}
			errCh <- err
			return
		}
		if a.logger != nil {
			a.logger.Info("kafka consumer stopped gracefully", "topic", topic, "group_id", groupID)
		}
	}()

	return errCh
}

func (a *ConsumerAdapter) Close() error {
	if a == nil || a.consumer == nil {
		return nil
	}
	err := a.consumer.Close()
	if err != nil {
		if a.logger != nil {
			a.logger.Error("internal.adapter.kafka.ConsumerAdapter.Close: close consumer failed", err)
		}
		return err
	}
	if a.logger != nil {
		a.logger.Info("internal.adapter.kafka.ConsumerAdapter.Close: consumer closed")
	}
	return nil
}
