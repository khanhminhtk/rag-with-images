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
	logPath        = "/home/minhtk/code/rag_imtotext_texttoim/worktree/service-rag/logs/app.log"
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

	ids := []string{
		"f0b13f53-0f15-4f1f-8ed1-8f8e3663c501", 
		"10001",                                
	}

	if err := store.DeleteByIDs(ctx, collectionName, ids); err != nil {
		panic(fmt.Errorf("delete by ids failed: %w", err))
	}

	fmt.Printf("deleted by ids successfully: %v\n", ids)

	results, err := store.Search(ctx, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameBM25,
		QueryText:      "transformer self-attention",
		Limit:          5,
		WithPayload:    true,
		Filter: &ports.Filter{
			Must: []ports.FieldCondition{{
				Key:      "doc_id",
				Operator: ports.MatchOperatorEqual,
				Value:    "slide_transformer_intro",
			}},
		},
	})
	if err != nil {
		panic(fmt.Errorf("verify search after delete by ids failed: %w", err))
	}

	fmt.Printf("remaining points for doc_id=slide_transformer_intro: %d\n", len(results))
}
