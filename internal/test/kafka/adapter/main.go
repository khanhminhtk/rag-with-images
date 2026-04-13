package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	kafkaAdapter "rag_imagetotext_texttoimage/internal/adapter/kafka"
	"rag_imagetotext_texttoimage/internal/application/ports"
	infraKafka "rag_imagetotext_texttoimage/internal/infra/kafka"
	"rag_imagetotext_texttoimage/internal/util"
)

type DemoEvent struct {
	Event   string `json:"event"`
	Message string `json:"message"`
	At      string `json:"at"`
}

func main() {
	appLogger, err := util.NewFileLogger(
		"logs/kafka_adapter_demo.log",
		slog.LevelInfo,
	)
	if err != nil {
		panic(err)
	}
	defer appLogger.Close()

	producer, consumer, err := kafkaAdapter.NewInfraAdapters(
		kafkaAdapter.InfraAdapterConfig{
			Brokers:     []string{"localhost:9092"},
			DialTimeout: 10 * time.Second,
			Publisher: infraKafka.PublisherConfig{
				RequiredAcks: -1,
			},
			Consumer: infraKafka.ConsumerConfig{
				MinBytes:       1,
				MaxBytes:       10e6,
				CommitInterval: time.Second,
				StartOffset:    -2,
			},
		},
		appLogger,
	)
	if err != nil {
		panic(err)
	}
	defer producer.Close()
	defer consumer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := consumer.Start(ctx, "demo-topic", "demo-group", func(ctx context.Context, msg ports.ConsumeMessage) error {
		return handleMessage(ctx, appLogger, producer, msg)
	})

	if err := producer.PublishJSON(ctx, "demo-topic", []byte("demo-key"), map[string]any{
		"event":   "demo_event",
		"message": "hello from adapter",
		"at":      time.Now().UTC().Format(time.RFC3339),
	}, map[string]string{
		"source": "internal/test/kafka/adapter",
	}); err != nil {
		panic(err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-time.After(2 * time.Second):
		appLogger.Info("demo finished")
	}
}

func handleMessage(ctx context.Context, appLogger util.Logger, producer *kafkaAdapter.ProducerAdapter, msg ports.ConsumeMessage) error {
	appLogger.Info("received kafka message",
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"key", string(msg.Message.Key),
		"value", string(msg.Message.Value),
	)

	var event DemoEvent
	if err := json.Unmarshal(msg.Message.Value, &event); err != nil {
		// Return error to stop consumer and avoid committing malformed data.
		return fmt.Errorf("invalid message format: %w", err)
	}

	switch event.Event {
	case "demo_event":
		// Place business logic here: call usecase, write DB, call external API...
		appLogger.Info("processing demo_event", "message", event.Message, "at", event.At)

		if strings.Contains(event.Message, "A") {
			reply := map[string]any{
				"event":   "demo_event_reply",
				"message": strings.ReplaceAll(event.Message, "A", "B"),
				"at":      time.Now().UTC().Format(time.RFC3339),
			}
			if err := producer.PublishJSON(ctx, "demo-topic-reply", []byte("reply-key"), reply, map[string]string{
				"source": "consumer-handleMessage",
			}); err != nil {
				return fmt.Errorf("publish reply message failed: %w", err)
			}
			appLogger.Info("published reply message",
				"reply_topic", "demo-topic-reply",
				"reply_message", reply["message"],
			)
		}
		return nil
	default:
		return fmt.Errorf("unsupported event type: %s", event.Event)
	}
}
