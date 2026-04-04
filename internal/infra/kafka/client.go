package kafka

import (
	"errors"
	"strings"
	"time"

	segmentio "github.com/segmentio/kafka-go"

	"rag_imagetotext_texttoimage/internal/util"
)

type KafkaConfig struct {
	Brokers     []string
	DialTimeout time.Duration
}

type KafkaClient struct {
	brokers []string
	dialer  *segmentio.Dialer
	appLogger  util.Logger
}

func NewKafkaClient(config KafkaConfig, appLogger util.Logger) (*KafkaClient, error) {
	if len(config.Brokers) == 0 {
		err := errors.New("internal.infra.kafka.client.NewKafkaClient: Kafka brokers are empty")
		return nil, err
	}

	cleanedBrokers := make([]string, 0, len(config.Brokers))
	for _, broker := range config.Brokers {
		b := strings.TrimSpace(broker)
		if b == "" {
			continue
		}
		cleanedBrokers = append(cleanedBrokers, b)
	}
	if len(cleanedBrokers) == 0 {
		return nil, errors.New("internal.infra.kafka.client.NewKafkaClient: Kafka brokers are invalid")
	}

	timeout := config.DialTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &KafkaClient{
		brokers: cleanedBrokers,
		dialer: &segmentio.Dialer{
			Timeout: timeout,
		},
		appLogger: appLogger,
	}, nil
}

func (c *KafkaClient) Brokers() []string {
	if c == nil {
		return nil
	}

	brokers := make([]string, len(c.brokers))
	copy(brokers, c.brokers)
	return brokers
}

func (c *KafkaClient) Dialer() *segmentio.Dialer {
	if c == nil {
		return nil
	}
	return c.dialer
}

func (c *KafkaClient) Logger() util.Logger {
	if c == nil {
		return nil
	}
	return c.appLogger
}
