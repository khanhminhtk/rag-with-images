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

	runDenseTextSearch(ctx, store)
	runDenseImageSearch(ctx, store)
	runLexicalSearch(ctx, store)
	runFilteredLexicalSearch(ctx, store)
}

func runDenseTextSearch(ctx context.Context, store *qdrant.PointStore) {
	results, err := store.Search(ctx, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameTextDense,
		Vector:         []float32{0.90, 0.10, 0.30, 0.80},
		Limit:          3,
		WithPayload:    true,
	})
	if err != nil {
		panic(fmt.Errorf("dense text search failed: %w", err))
	}
	printResults("dense text search", results)
}

func runDenseImageSearch(ctx context.Context, store *qdrant.PointStore) {
	results, err := store.Search(ctx, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameImageDense,
		Vector:         []float32{0.12, 0.85, 0.45, 0.50},
		Limit:          3,
		WithPayload:    true,
	})
	if err != nil {
		panic(fmt.Errorf("dense image search failed: %w", err))
	}
	printResults("dense image search", results)
}

func runLexicalSearch(ctx context.Context, store *qdrant.PointStore) {
	results, err := store.Search(ctx, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameBM25,
		QueryText:      "transformer architecture encoder decoder residual",
		Limit:          3,
		WithPayload:    true,
	})
	if err != nil {
		panic(fmt.Errorf("lexical bm25 search failed: %w", err))
	}
	printResults("lexical bm25 search", results)
}

func runFilteredLexicalSearch(ctx context.Context, store *qdrant.PointStore) {
	results, err := store.Search(ctx, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameBM25,
		QueryText:      "OCR retrieval mRAG",
		Limit:          3,
		WithPayload:    true,
		Filter: &ports.Filter{
			Must: []ports.FieldCondition{
				{
					Key:      "lang",
					Operator: ports.MatchOperatorEqual,
					Value:    "vi",
				},
			},
		},
	})
	if err != nil {
		panic(fmt.Errorf("filtered lexical search failed: %w", err))
	}
	printResults("filtered lexical search (lang=vi)", results)
}

func printResults(title string, results []ports.SearchResult) {
	fmt.Printf("\n=== %s ===\n", title)
	if len(results) == 0 {
		fmt.Println("no results")
		return
	}
	for idx, r := range results {
		payload := r.Point.Payload
		fmt.Printf(
			"%d) id=%s score=%.4f doc_id=%s page=%d unit_type=%s lang=%s section=%q\n",
			idx+1,
			r.Point.ID,
			r.Score,
			payload.DocID,
			payload.Page,
			payload.UnitType,
			payload.Lang,
			payload.SectionTitle,
		)
	}
}
