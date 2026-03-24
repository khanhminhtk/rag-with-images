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
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: config.Host,
		Port: config.Port,
	})
	if err != nil {
		appLogger.Error("Failed to create qdrant client", err)
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.HealthCheck(ctx)
	if err != nil {
		_ = client.Close()
		appLogger.Error("health check failed", err)
		return nil, err
	}

	appLogger.Info("Qdrant client created successfully", "version", health.GetVersion())
	return &Client{
		raw: client,
	}, nil
}

func (c *Client) Raw() *qdrant.Client {
	return c.raw
}

func (c *Client) Close() error {
	if c == nil || c.raw == nil {
		return nil
	}

	if err := c.raw.Close(); err != nil {
		return fmt.Errorf("close qdrant client: %w", err)
	}
	return nil
}
