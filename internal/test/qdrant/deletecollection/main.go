package main

import (
	"context"
	"log/slog"

	// "rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"
)

func getclient(appLogger util.Logger) *qdrant.Client {
	config := qdrant.Config{
		Host: "localhost",
		Port: 6334,
	}

	client, err := qdrant.NewClient(config, appLogger)
	if err != nil {
		panic(err)
	}

	return client

}

func main() {
	ctx := context.Background()

	appLogger, err := util.NewFileLogger(
		"logs/app.log",
		slog.LevelInfo,
	)

	if err != nil {
		panic(err)
	}
	client := getclient(appLogger)

	qdrantStore := qdrant.NewCollectionStore(
		client.Raw(),
		appLogger,
	)

	qdrantStore.DeleteCollection(ctx, "test")
}