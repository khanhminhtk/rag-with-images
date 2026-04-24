package main

import (
	"context"
	"fmt"
	"log/slog"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"
)

const (
	collectionName = "test"
	logPath        = "logs/app.log"
)

func getClient(appLogger util.Logger) *qdrant.Client {
	client, err := qdrant.NewClient(qdrant.Config{
		Host: "localhost",
		Port: 6334,
	}, appLogger)
	if err != nil {
		panic(err)
	}
	return client
}

func main() {
	ctx := context.Background()

	appLogger, err := util.NewFileLogger(logPath, slog.LevelInfo)
	if err != nil {
		panic(err)
	}
	defer appLogger.Close()

	client := getClient(appLogger)
	defer client.Close()

	store := qdrant.NewPointStore(client.Raw(), appLogger)

	filter := ports.Filter{
		Must: []ports.FieldCondition{
			{
				Key:      "lang",
				Operator: ports.MatchOperatorEqual,
				Value:    "vi",
			},
		},
	}

	if err := store.DeleteByFilter(ctx, collectionName, filter); err != nil {
		panic(fmt.Errorf("delete by filter failed: %w", err))
	}

	fmt.Println("deleted points by filter successfully: lang=vi")

	results, err := store.Search(ctx, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameBM25,
		QueryText:      "OCR retrieval mRAG",
		Limit:          5,
		WithPayload:    true,
		Filter:         &filter,
	})
	if err != nil {
		panic(fmt.Errorf("verify search after delete by filter failed: %w", err))
	}

	fmt.Printf("remaining points with lang=vi: %d\n", len(results))
}
