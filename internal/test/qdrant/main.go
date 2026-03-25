package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"
	"rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"

	qdrantapi "github.com/qdrant/go-client/qdrant"
)

const (
	collectionName = "test_e2e_qdrant"
	logPath        = "/home/minhtk/code/rag_imtotext_texttoim/worktree/service-rag/logs/app.log"
)

func main() {
	ctx := context.Background()

	appLogger, err := util.NewFileLogger(logPath, slog.LevelInfo)
	mustNoErr("create app logger", err)
	defer appLogger.Close()

	client, err := qdrant.NewClient(qdrant.Config{Host: "localhost", Port: 6334}, appLogger)
	mustNoErr("create qdrant client", err)
	defer client.Close()

	collectionStore := qdrant.NewCollectionStore(client.Raw(), appLogger)
	pointStore := qdrant.NewPointStore(client.Raw(), appLogger)

	resetCollectionIfExists(ctx, collectionStore)

	fmt.Println("[1/6] ensure collection")
	mustNoErr("ensure collection", collectionStore.EnsureCollection(ctx, buildSchema()))
	mustCollectionExists(ctx, collectionStore, true)

	fmt.Println("[2/6] upsert points + verify count")
	points := buildTestPoints()
	mustNoErr("upsert points", pointStore.Upsert(ctx, collectionName, points))
	mustCount(ctx, client.Raw(), nil, uint64(len(points)))

	fmt.Println("[3/6] search + verify business case")
	mustSearchLen(ctx, pointStore, ports.SearchQuery{
		CollectionName: collectionName,
		VectorName:     ports.VectorNameBM25,
		QueryText:      "mRAG OCR retrieval",
		Limit:          10,
		WithPayload:    true,
		Filter: &ports.Filter{
			Must: []ports.FieldCondition{{
				Key:      "lang",
				Operator: ports.MatchOperatorEqual,
				Value:    "vi",
			}},
		},
	}, 1)

	fmt.Println("[4/6] delete by filter (lang=vi) + verify count")
	mustNoErr("delete by filter", pointStore.DeleteByFilter(ctx, collectionName, ports.Filter{
		Must: []ports.FieldCondition{{
			Key:      "lang",
			Operator: ports.MatchOperatorEqual,
			Value:    "vi",
		}},
	}))
	mustCount(ctx, client.Raw(), nil, uint64(len(points)-1))

	fmt.Println("[5/6] delete by ids (all remaining) + verify count")
	remainingIDs := []string{
		"f0b13f53-0f15-4f1f-8ed1-8f8e3663c501",
		"10001",
		"f0b13f53-0f15-4f1f-8ed1-8f8e3663c503",
		"f0b13f53-0f15-4f1f-8ed1-8f8e3663c504",
	}
	mustNoErr("delete by ids", pointStore.DeleteByIDs(ctx, collectionName, remainingIDs))
	mustCount(ctx, client.Raw(), nil, 0)

	fmt.Println("[6/6] delete collection + verify exists=false")
	mustNoErr("delete collection", collectionStore.DeleteCollection(ctx, collectionName))
	mustCollectionExists(ctx, collectionStore, false)

	fmt.Println("PASS: full qdrant e2e pipeline completed")
}

func resetCollectionIfExists(ctx context.Context, collectionStore *qdrant.CollectionStore) {
	exists, err := collectionStore.CollectionExists(ctx, collectionName)
	mustNoErr("check collection exists before reset", err)
	if exists {
		mustNoErr("delete existing collection before test", collectionStore.DeleteCollection(ctx, collectionName))
	}
}

func mustCollectionExists(ctx context.Context, collectionStore *qdrant.CollectionStore, expected bool) {
	exists, err := collectionStore.CollectionExists(ctx, collectionName)
	mustNoErr("check collection exists", err)
	if exists != expected {
		panic(fmt.Errorf("collection exists mismatch: expected=%v got=%v", expected, exists))
	}
}

func mustCount(ctx context.Context, raw *qdrantapi.Client, filter *qdrantapi.Filter, expected uint64) {
	exact := true
	count, err := raw.Count(ctx, &qdrantapi.CountPoints{
		CollectionName: collectionName,
		Filter:         filter,
		Exact:          &exact,
	})
	mustNoErr("count points", err)
	if count != expected {
		panic(fmt.Errorf("point count mismatch: expected=%d got=%d", expected, count))
	}
}

func mustSearchLen(ctx context.Context, pointStore *qdrant.PointStore, query ports.SearchQuery, expected int) {
	results, err := pointStore.Search(ctx, query)
	mustNoErr("search points", err)
	if len(results) != expected {
		panic(fmt.Errorf("search result mismatch: expected=%d got=%d", expected, len(results)))
	}
}

func mustNoErr(step string, err error) {
	if err != nil {
		panic(fmt.Errorf("%s: %w", step, err))
	}
}

func buildSchema() ports.CollectionSchema {
	return ports.CollectionSchema{
		Name: collectionName,
		Vectors: []ports.CollectionVectorConfig{
			{Name: ports.VectorNameTextDense, Size: 4, Distance: ports.DistanceCosine},
			{Name: ports.VectorNameImageDense, Size: 4, Distance: ports.DistanceCosine},
		},
		Shards:            1,
		ReplicationFactor: 1,
		OnDiskPayload:     true,
		OptimizersMemmap:  true,
	}
}

func buildTestPoints() []domain.PointObject {
	now := time.Now().UTC()
	return []domain.PointObject{
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c501",
			Vector: domain.VectorObject{
				TextDense: []float32{0.91, 0.12, 0.33, 0.77},
			},
			Payload: domain.PointPayload{
				DocID:        "doc_1",
				Page:         1,
				Modality:     "text",
				UnitType:     "paragraph",
				Text:         "Transformer uses self-attention for sequence modeling.",
				OCRText:      "Transformer architecture overview",
				SectionTitle: "Introduction",
				Lang:         "en",
				ChunkIndex:   0,
				TokenCount:   20,
				Keywords:     []string{"transformer", "attention"},
				CreatedAt:    now,
			},
		},
		{
			ID: "10001",
			Vector: domain.VectorObject{
				TextDense: []float32{0.70, 0.30, 0.21, 0.66},
			},
			Payload: domain.PointPayload{
				DocID:        "doc_2",
				Page:         2,
				Modality:     "text",
				UnitType:     "paragraph",
				Text:         "RAG retrieval benchmark and latency details.",
				OCRText:      "retrieve and rerank timings",
				SectionTitle: "Performance",
				Lang:         "en",
				ChunkIndex:   1,
				TokenCount:   16,
				Keywords:     []string{"rag", "latency"},
				CreatedAt:    now,
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c503",
			Vector: domain.VectorObject{
				TextDense: []float32{0.73, 0.41, 0.62, 0.26},
			},
			Payload: domain.PointPayload{
				DocID:        "doc_3",
				Page:         3,
				Modality:     "text",
				UnitType:     "caption",
				Text:         "Encoder and decoder stacks with residual blocks.",
				OCRText:      "Transformer encoder decoder",
				SectionTitle: "Architecture Details",
				Lang:         "en",
				ChunkIndex:   2,
				TokenCount:   24,
				Keywords:     []string{"encoder", "decoder"},
				CreatedAt:    now,
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c504",
			Vector: domain.VectorObject{
				ImageDense: []float32{0.10, 0.88, 0.44, 0.55},
			},
			Payload: domain.PointPayload{
				DocID:        "doc_4",
				Page:         4,
				Modality:     "image",
				UnitType:     "figure",
				ImagePath:    "data/figure.png",
				SectionTitle: "Model Figure",
				Lang:         "en",
				ChunkIndex:   3,
				TokenCount:   0,
				Keywords:     []string{"figure", "model"},
				CreatedAt:    now,
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c505",
			Vector: domain.VectorObject{
				TextDense: []float32{0.54, 0.66, 0.13, 0.47},
			},
			Payload: domain.PointPayload{
				DocID:        "doc_vi",
				Page:         5,
				Modality:     "text",
				UnitType:     "paragraph",
				Text:         "He thong mRAG ket hop OCR va truy xuat.",
				OCRText:      "mRAG OCR retrieval",
				SectionTitle: "Tong quan",
				Lang:         "vi",
				ChunkIndex:   4,
				TokenCount:   21,
				Keywords:     []string{"mRAG", "OCR", "retrieval"},
				CreatedAt:    now,
			},
		},
	}
}
