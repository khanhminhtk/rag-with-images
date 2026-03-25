package qdrant

import (
	"strconv"
	"time"

	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"

	"github.com/qdrant/go-client/qdrant"
)

func pointIDToString(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	switch v := id.GetPointIdOptions().(type) {
	case *qdrant.PointId_Uuid:
		return v.Uuid
	case *qdrant.PointId_Num:
		return strconv.FormatUint(v.Num, 10)
	default:
		return ""
	}
}

func payloadFromQdrant(payload map[string]*qdrant.Value) domain.PointPayload {
	if len(payload) == 0 {
		return domain.PointPayload{}
	}

	out := domain.PointPayload{}
	if v, ok := getString(payload, "doc_id"); ok {
		out.DocID = v
	}
	if v, ok := getString(payload, "source_path"); ok {
		out.SourcePath = v
	}
	if v, ok := getInt(payload, "page"); ok {
		out.Page = v
	}
	if v, ok := getString(payload, "modality"); ok {
		out.Modality = v
	}
	if v, ok := getString(payload, "unit_type"); ok {
		out.UnitType = v
	}
	if v, ok := getString(payload, "text"); ok {
		out.Text = v
	}
	if v, ok := getString(payload, "ocr_text"); ok {
		out.OCRText = v
	}
	if v, ok := getString(payload, "image_path"); ok {
		out.ImagePath = v
	}
	if v, ok := getString(payload, "section_title"); ok {
		out.SectionTitle = v
	}
	if v, ok := getString(payload, "lang"); ok {
		out.Lang = v
	}
	if v, ok := getBool(payload, "has_table"); ok {
		out.HasTable = v
	}
	if v, ok := getBool(payload, "has_figure"); ok {
		out.HasFigure = v
	}
	if v, ok := getString(payload, "parent_id"); ok {
		out.ParentID = v
	}
	if v, ok := getInt(payload, "chunk_index"); ok {
		out.ChunkIndex = v
	}
	if v, ok := getInt(payload, "token_count"); ok {
		out.TokenCount = v
	}
	if v, ok := getStringSlice(payload, "keywords"); ok {
		out.Keywords = v
	}
	if v, ok := getTime(payload, "created_at"); ok {
		out.CreatedAt = v
	}
	if v, ok := getBBox(payload, "bbox"); ok {
		out.BBox = v
	}

	return out
}

func getString(payload map[string]*qdrant.Value, key string) (string, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return "", false
	}
	if s, ok := v.GetKind().(*qdrant.Value_StringValue); ok {
		return s.StringValue, true
	}
	return "", false
}

func getBool(payload map[string]*qdrant.Value, key string) (bool, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return false, false
	}
	if b, ok := v.GetKind().(*qdrant.Value_BoolValue); ok {
		return b.BoolValue, true
	}
	return false, false
}

func getInt(payload map[string]*qdrant.Value, key string) (int, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return 0, false
	}
	switch value := v.GetKind().(type) {
	case *qdrant.Value_IntegerValue:
		return int(value.IntegerValue), true
	case *qdrant.Value_DoubleValue:
		return int(value.DoubleValue), true
	default:
		return 0, false
	}
}

func getStringSlice(payload map[string]*qdrant.Value, key string) ([]string, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return nil, false
	}
	list, ok := v.GetKind().(*qdrant.Value_ListValue)
	if !ok || list.ListValue == nil {
		return nil, false
	}
	out := make([]string, 0, len(list.ListValue.Values))
	for _, item := range list.ListValue.Values {
		if item == nil {
			continue
		}
		if s, ok := item.GetKind().(*qdrant.Value_StringValue); ok {
			out = append(out, s.StringValue)
		}
	}
	return out, len(out) > 0
}

func getTime(payload map[string]*qdrant.Value, key string) (time.Time, bool) {
	s, ok := getString(payload, key)
	if !ok || s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func getBBox(payload map[string]*qdrant.Value, key string) (*domain.BoundingBox, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return nil, false
	}
	sv, ok := v.GetKind().(*qdrant.Value_StructValue)
	if !ok || sv.StructValue == nil {
		return nil, false
	}

	fields := sv.StructValue.GetFields()
	x1, okX1 := getInt(fields, "x1")
	y1, okY1 := getInt(fields, "y1")
	x2, okX2 := getInt(fields, "x2")
	y2, okY2 := getInt(fields, "y2")
	if !okX1 || !okY1 || !okX2 || !okY2 {
		return nil, false
	}

	return &domain.BoundingBox{
		X1: x1,
		Y1: y1,
		X2: x2,
		Y2: y2,
	}, true
}
