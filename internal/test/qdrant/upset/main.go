package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"
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
	const collectionName = "test"

	appLogger, err := util.NewFileLogger(
		"/home/minhtk/code/rag_imtotext_texttoim/worktree/service-rag/logs/app.log",
		slog.LevelInfo,
	)

	if err != nil {
		panic(err)
	}

	client := getclient(appLogger)
	qdrantpointStore := qdrant.NewPointStore(
		client.Raw(),
		appLogger,
	)

	points := buildFakePoints()

	if err := qdrantpointStore.Upsert(ctx, collectionName, points); err != nil {
		panic(err)
	}

	fmt.Printf("upserted %d points into collection %q\n", len(points), collectionName)
}

func buildFakePoints() []domain.PointObject {
	now := time.Now().UTC()
	imagePath := "/home/minhtk/code/rag_imtotext_texttoim/worktree/service-rag/data/transformer_model_architecture.png"

	return []domain.PointObject{
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c501",
			Vector: domain.VectorObject{
				TextDense: []float32{0.91, 0.12, 0.33, 0.77},
			},
			Payload: domain.PointPayload{
				DocID:        "slide_transformer_intro",
				SourcePath:   "data/docs/transformer_intro_slides.pdf",
				Page:         1,
				Modality:     "text",
				UnitType:     "paragraph",
				Text:         "Transformer replaces recurrence with self-attention for sequence modeling.",
				OCRText:      "Transformer architecture overview",
				SectionTitle: "Introduction",
				Lang:         "en",
				HasTable:     false,
				HasFigure:    false,
				ParentID:     "doc-slide-001",
				ChunkIndex:   0,
				TokenCount:   20,
				Keywords:     []string{"transformer", "self-attention", "nlp"},
				CreatedAt:    now,
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c502",
			Vector: domain.VectorObject{
				ImageDense: []float32{0.10, 0.88, 0.44, 0.55},
			},
			Payload: domain.PointPayload{
				DocID:        "slide_transformer_intro",
				SourcePath:   "data/docs/transformer_intro_slides.pdf",
				Page:         2,
				Modality:     "image",
				UnitType:     "figure",
				ImagePath:    imagePath,
				SectionTitle: "Model Architecture",
				Lang:         "en",
				HasTable:     false,
				HasFigure:    true,
				ParentID:     "doc-slide-001",
				ChunkIndex:   1,
				TokenCount:   0,
				Keywords:     []string{"architecture", "encoder", "decoder"},
				CreatedAt:    now,
				BBox: &domain.BoundingBox{
					X1: 120,
					Y1: 160,
					X2: 980,
					Y2: 1180,
				},
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c503",
			Vector: domain.VectorObject{
				TextDense:  []float32{0.73, 0.41, 0.62, 0.26},
				ImageDense: []float32{0.68, 0.22, 0.52, 0.31},
			},
			Payload: domain.PointPayload{
				DocID:        "slide_transformer_intro",
				SourcePath:   "data/docs/transformer_intro_slides.pdf",
				Page:         3,
				Modality:     "multi",
				UnitType:     "caption",
				Text:         "Figure shows stacked encoder and decoder blocks with residual connections.",
				OCRText:      "Figure: Transformer model architecture",
				ImagePath:    imagePath,
				SectionTitle: "Architecture Details",
				Lang:         "en",
				HasTable:     false,
				HasFigure:    true,
				ParentID:     "doc-slide-001",
				ChunkIndex:   2,
				TokenCount:   24,
				Keywords:     []string{"residual", "layernorm", "attention"},
				CreatedAt:    now,
				BBox: &domain.BoundingBox{
					X1: 100,
					Y1: 120,
					X2: 1000,
					Y2: 1240,
				},
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c504",
			Vector: domain.VectorObject{
				TextDense: []float32{0.22, 0.19, 0.81, 0.67},
			},
			Payload: domain.PointPayload{
				DocID:        "manual_rag_system",
				SourcePath:   "data/docs/rag_system_manual.pdf",
				Page:         5,
				Modality:     "text",
				UnitType:     "table",
				Text:         "Latency benchmarks for retrieval and reranking stages.",
				OCRText:      "Stage p50 p95\nretrieve 120 310\nrerank 90 180",
				SectionTitle: "Performance Table",
				Lang:         "en",
				HasTable:     true,
				HasFigure:    false,
				ParentID:     "doc-manual-001",
				ChunkIndex:   0,
				TokenCount:   18,
				Keywords:     []string{"latency", "p95", "benchmark"},
				CreatedAt:    now,
			},
		},
		{
			ID: "f0b13f53-0f15-4f1f-8ed1-8f8e3663c505",
			Vector: domain.VectorObject{
				TextDense: []float32{0.54, 0.66, 0.13, 0.47},
			},
			Payload: domain.PointPayload{
				DocID:        "lecture_vi_multimodal_rag",
				SourcePath:   "data/docs/lecture_vi_multimodal_rag.pdf",
				Page:         7,
				Modality:     "text",
				UnitType:     "paragraph",
				Text:         "He thong mRAG ket hop OCR va embedding hinh anh de tang do chinh xac truy xuat.",
				OCRText:      "mRAG ket hop text va image",
				SectionTitle: "Tong quan he thong",
				Lang:         "vi",
				HasTable:     false,
				HasFigure:    false,
				ParentID:     "doc-lecture-vi",
				ChunkIndex:   3,
				TokenCount:   21,
				Keywords:     []string{"mRAG", "OCR", "retrieval"},
				CreatedAt:    now,
			},
		},
	}
}
