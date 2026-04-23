package ports

import "context"

type KafkaMessage struct {
	Key     []byte
	Value   []byte
	Headers map[string]string
}

type PublishMessageInput struct {
	Topic   string
	Message KafkaMessage
}

type ConsumeMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Lag       int64
	Message   KafkaMessage
}

type MessageHandler func(ctx context.Context, msg ConsumeMessage) error

type KafkaPublisher interface {
	Publish(ctx context.Context, input PublishMessageInput) error
	Close() error
}

type KafkaConsumer interface {
	Consume(ctx context.Context, topic string, groupID string, handler MessageHandler) error
	Close() error
}
