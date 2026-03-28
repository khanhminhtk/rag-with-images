package domain

import (
	"fmt"
	"time"
)

type BoundingBox struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

type VectorObject struct {
	TextDense  []float32 `json:"text_dense,omitempty"`
	ImageDense []float32 `json:"image_dense,omitempty"`
}

func (v VectorObject) IsEmpty() bool {
	return len(v.TextDense) == 0 && len(v.ImageDense) == 0
}

type PointPayload struct {
	DocID        string       `json:"doc_id"`
	SourcePath   string       `json:"source_path,omitempty"`
	Page         int          `json:"page"`
	Modality     string       `json:"modality,omitempty"`
	UnitType     string       `json:"unit_type"`
	Text         string       `json:"text,omitempty"`
	OCRText      string       `json:"ocr_text,omitempty"`
	BBox         *BoundingBox `json:"bbox,omitempty"`
	ImagePath    string       `json:"image_path,omitempty"`
	SectionTitle string       `json:"section_title,omitempty"`
	Lang         string       `json:"lang,omitempty"`
	HasTable     bool         `json:"has_table,omitempty"`
	HasFigure    bool         `json:"has_figure,omitempty"`
	ParentID     string       `json:"parent_id,omitempty"`
	ChunkIndex   int          `json:"chunk_index,omitempty"`
	TokenCount   int          `json:"token_count,omitempty"`
	Keywords     []string     `json:"keywords,omitempty"`
	CreatedAt    time.Time    `json:"created_at,omitempty"`
}

type PointObject struct {
	ID      string       `json:"id"`
	Vector  VectorObject `json:"vector"`
	Payload PointPayload `json:"payload"`
}

func NewPointObject(id string, vector VectorObject, payload PointPayload) (*PointObject, error) {
	point := &PointObject{
		ID:      id,
		Vector:  vector,
		Payload: payload,
	}

	if err := point.Validate(); err != nil {
		return nil, err
	}

	return point, nil
}

func (p *PointObject) Validate() error {
	if p == nil {
		return fmt.Errorf("point is nil")
	}

	if p.ID == "" {
		return fmt.Errorf("point id is required")
	}

	if p.Vector.IsEmpty() {
		return fmt.Errorf("at least one vector is required")
	}

	if p.Payload.DocID == "" {
		return fmt.Errorf("payload doc_id is required")
	}

	if p.Payload.UnitType == "" {
		return fmt.Errorf("payload unit_type is required")
	}

	if p.Payload.Page < 0 {
		return fmt.Errorf("payload page must be non-negative")
	}

	return nil
}
