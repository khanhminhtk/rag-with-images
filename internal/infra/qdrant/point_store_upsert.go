package qdrant

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"

	"github.com/qdrant/go-client/qdrant"
)

func (p *PointStore) Upsert(ctx context.Context, collectionName string, points []domain.PointObject) error {
	source := qdrantSource("PointStore.Upsert")
	p.appLogger.Debug("upsert points started", "source", source, "collection", collectionName, "points", len(points))

	if collectionName == "" {
		err := fmt.Errorf("collection name is required")
		p.appLogger.Error("upsert validation failed", err, "source", source)
		return fmt.Errorf("%s: %w", source, err)
	}
	if len(points) == 0 {
		err := fmt.Errorf("points are required")
		p.appLogger.Error("upsert validation failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: %w", source, err)
	}

	qdrantPoints := make([]*qdrant.PointStruct, 0, len(points))

	for _, point := range points {
		payload := map[string]any{
			"doc_id":      point.Payload.DocID,
			"page":        point.Payload.Page,
			"unit_type":   point.Payload.UnitType,
			"has_table":   point.Payload.HasTable,
			"has_figure":  point.Payload.HasFigure,
			"chunk_index": point.Payload.ChunkIndex,
			"token_count": point.Payload.TokenCount,
			"created_at":  point.Payload.CreatedAt.Format(time.RFC3339),
		}
		if point.Payload.SourcePath != "" {
			payload["source_path"] = point.Payload.SourcePath
		}
		if point.Payload.Modality != "" {
			payload["modality"] = point.Payload.Modality
		}
		if point.Payload.Text != "" {
			payload["text"] = point.Payload.Text
		}
		if point.Payload.OCRText != "" {
			payload["ocr_text"] = point.Payload.OCRText
		}
		if point.Payload.ImagePath != "" {
			payload["image_path"] = point.Payload.ImagePath
		}
		if point.Payload.SectionTitle != "" {
			payload["section_title"] = point.Payload.SectionTitle
		}
		if point.Payload.Lang != "" {
			payload["lang"] = point.Payload.Lang
		}
		if point.Payload.ParentID != "" {
			payload["parent_id"] = point.Payload.ParentID
		}
		if len(point.Payload.Keywords) > 0 {
			keywords := make([]any, 0, len(point.Payload.Keywords))
			for _, kw := range point.Payload.Keywords {
				keywords = append(keywords, kw)
			}
			payload["keywords"] = keywords
		}
		if point.Payload.BBox != nil {
			payload["bbox"] = map[string]any{
				"x1": point.Payload.BBox.X1,
				"y1": point.Payload.BBox.Y1,
				"x2": point.Payload.BBox.X2,
				"y2": point.Payload.BBox.Y2,
			}
		}

		vectorMap := make(map[string]*qdrant.Vector)
		if len(point.Vector.TextDense) > 0 {
			vectorMap[vectorNameTextDense] = qdrant.NewVectorDense(point.Vector.TextDense)
		}
		if len(point.Vector.ImageDense) > 0 {
			vectorMap[vectorNameImageDense] = qdrant.NewVectorDense(point.Vector.ImageDense)
		}
		if lexicalText := buildBM25Text(point.Payload); lexicalText != "" {
			vectorMap[vectorNameBM25] = qdrant.NewVectorDocument(&qdrant.Document{
				Model: vectorModelBM25,
				Text:  lexicalText,
			})
		}

		if len(vectorMap) == 0 {
			err := fmt.Errorf("point %s has no vectors", point.ID)
			p.appLogger.Error("upsert validation failed", err, "source", source, "collection", collectionName, "point_id", point.ID)
			return fmt.Errorf("%s: %w", source, err)
		}

		qdrantPoints = append(qdrantPoints, &qdrant.PointStruct{
			Id:      toQdrantPointID(point.ID),
			Vectors: qdrant.NewVectorsMap(vectorMap),
			Payload: qdrant.NewValueMap(payload),
		})
	}

	wait := true
	_, err := p.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points:         qdrantPoints,
	})
	if err != nil {
		p.appLogger.Error("upsert points failed", err, "source", source, "collection", collectionName, "points", len(points))
		return fmt.Errorf("%s: qdrant upsert failed: %w", source, err)
	}

	p.appLogger.Info("upsert points success", "source", source, "collection", collectionName, "points", len(points))
	return nil
}

func buildBM25Text(payload domain.PointPayload) string {
	parts := make([]string, 0, 4)
	if text := strings.TrimSpace(payload.SectionTitle); text != "" {
		parts = append(parts, text)
	}
	if text := strings.TrimSpace(payload.Text); text != "" {
		parts = append(parts, text)
	}
	if text := strings.TrimSpace(payload.OCRText); text != "" {
		parts = append(parts, text)
	}
	if len(payload.Keywords) > 0 {
		keywords := strings.TrimSpace(strings.Join(payload.Keywords, " "))
		if keywords != "" {
			parts = append(parts, keywords)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
