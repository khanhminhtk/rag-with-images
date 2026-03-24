package qdrant

import (
	"context"
	"fmt"
	"time"

	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"

	"github.com/qdrant/go-client/qdrant"
)

func (q *PointStore) Upsert(ctx context.Context, collectionName string, points []domain.PointObject) error {
	qdrantPoints := make([]*qdrant.PointStruct, 0, len(points))

	for _, p := range points {
		payload := map[string]any{
			"doc_id":      p.Payload.DocID,
			"page":        p.Payload.Page,
			"unit_type":   p.Payload.UnitType,
			"has_table":   p.Payload.HasTable,
			"has_figure":  p.Payload.HasFigure,
			"chunk_index": p.Payload.ChunkIndex,
			"token_count": p.Payload.TokenCount,
			"created_at":  p.Payload.CreatedAt.Format(time.RFC3339),
		}
		if p.Payload.SourcePath != "" {
			payload["source_path"] = p.Payload.SourcePath
		}
		if p.Payload.Modality != "" {
			payload["modality"] = p.Payload.Modality
		}
		if p.Payload.Text != "" {
			payload["text"] = p.Payload.Text
		}
		if p.Payload.OCRText != "" {
			payload["ocr_text"] = p.Payload.OCRText
		}
		if p.Payload.ImagePath != "" {
			payload["image_path"] = p.Payload.ImagePath
		}
		if p.Payload.SectionTitle != "" {
			payload["section_title"] = p.Payload.SectionTitle
		}
		if p.Payload.Lang != "" {
			payload["lang"] = p.Payload.Lang
		}
		if p.Payload.ParentID != "" {
			payload["parent_id"] = p.Payload.ParentID
		}
		if len(p.Payload.Keywords) > 0 {
			keywords := make([]any, 0, len(p.Payload.Keywords))
			for _, kw := range p.Payload.Keywords {
				keywords = append(keywords, kw)
			}
			payload["keywords"] = keywords
		}
		if p.Payload.BBox != nil {
			payload["bbox"] = map[string]any{
				"x1": p.Payload.BBox.X1,
				"y1": p.Payload.BBox.Y1,
				"x2": p.Payload.BBox.X2,
				"y2": p.Payload.BBox.Y2,
			}
		}

		vectorMap := make(map[string]*qdrant.Vector)
		if len(p.Vector.TextDense) > 0 {
			vectorMap["text_dense"] = qdrant.NewVectorDense(p.Vector.TextDense)
		}
		if len(p.Vector.ImageDense) > 0 {
			vectorMap["image_dense"] = qdrant.NewVectorDense(p.Vector.ImageDense)
		}

		if len(vectorMap) == 0 {
			err := fmt.Errorf("point %s has no vectors", p.ID)
			q.appLogger.Error("upsert validation failed", err, "collection", collectionName, "point_id", p.ID)
			return err
		}

		qdrantPoints = append(qdrantPoints, &qdrant.PointStruct{
			Id:      qdrant.NewID(p.ID),
			Vectors: qdrant.NewVectorsMap(vectorMap),
			Payload: qdrant.NewValueMap(payload),
		})
	}

	wait := true
	_, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points:         qdrantPoints,
	})
	if err != nil {
		q.appLogger.Error("upsert points failed", err, "collection", collectionName, "points", len(points))
		return fmt.Errorf("qdrant upsert failed: %w", err)
	}

	q.appLogger.Info("upsert points success", "collection", collectionName, "points", len(points))
	return nil
}
