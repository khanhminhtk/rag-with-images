package qdrant

import (
	"context"
	"fmt"
	"rag_imagetotext_texttoimage/internal/util"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

type Config struct {
	Host string
	Port int
}

type Client struct {
	raw *qdrant.Client
}

func NewClient(config Config, appLogger util.Logger) (*Client, error) {
	source := qdrantSource("NewClient")
	appLogger.Debug("creating qdrant client", "source", source, "host", config.Host, "port", config.Port)

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		appLogger.Error("create qdrant client failed", err, "source", source, "host", config.Host, "port", config.Port)
		return nil, fmt.Errorf("%s: create qdrant client failed: %w", source, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.HealthCheck(ctx)
	if err != nil {
		_ = client.Close()
		appLogger.Error("qdrant health check failed", err, "source", source, "host", config.Host, "port", config.Port)
		return nil, fmt.Errorf("%s: health check failed: %w", source, err)
	}

	appLogger.Info("qdrant client created successfully", "source", source, "version", health.GetVersion())
	return &Client{
		raw: client,
	}, nil
}

func (c *Client) Raw() *qdrant.Client {
	return c.raw
}

func (c *Client) Close() error {
	source := qdrantSource("Client.Close")
	if c == nil || c.raw == nil {
		return nil
	}

	if err := c.raw.Close(); err != nil {
		return fmt.Errorf("%s: close qdrant client: %w", source, err)
	}
	return nil
}
