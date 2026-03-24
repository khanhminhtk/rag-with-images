package qdrant

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"

	"github.com/qdrant/go-client/qdrant"
)

func (q *PointStore) Search(ctx context.Context, query ports.SearchQuery) ([]ports.SearchResult, error) {
	if query.CollectionName == "" {
		return nil, fmt.Errorf("collection name is required")
	}
	if len(query.Vector) == 0 {
		return nil, fmt.Errorf("query vector is required")
	}
	if query.Limit == 0 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}

	req := &qdrant.QueryPoints{
		CollectionName: query.CollectionName,
		Query:          qdrant.NewQuery(query.Vector...),
		Limit:          qdrant.PtrOf(query.Limit),
		ScoreThreshold: query.ScoreThreshold,
		WithPayload:    qdrant.NewWithPayload(query.WithPayload),
	}
	if query.VectorName != "" {
		req.Using = qdrant.PtrOf(query.VectorName)
	}
	if query.Filter != nil && !query.Filter.IsEmpty() {
		filter, err := toQdrantFilter(query.Filter)
		if err != nil {
			return nil, err
		}
		req.Filter = filter
	}

	points, err := q.client.Query(ctx, req)
	if err != nil {
		q.appLogger.Error("query points failed", err, "collection", query.CollectionName)
		return nil, fmt.Errorf("qdrant query failed: %w", err)
	}

	results := make([]ports.SearchResult, 0, len(points))
	for _, p := range points {
		point := &domain.PointObject{
			ID: pointIDToString(p.GetId()),
		}
		if query.WithPayload {
			point.Payload = payloadFromQdrant(p.GetPayload())
		}
		results = append(results, ports.SearchResult{
			Point: point,
			Score: p.GetScore(),
		})
	}

	return results, nil
}

func toQdrantFilter(f *ports.Filter) (*qdrant.Filter, error) {
	if f == nil || f.IsEmpty() {
		return nil, nil
	}

	must, err := toQdrantConditions(f.Must)
	if err != nil {
		return nil, fmt.Errorf("must: %w", err)
	}
	should, err := toQdrantConditions(f.Should)
	if err != nil {
		return nil, fmt.Errorf("should: %w", err)
	}
	mustNot, err := toQdrantConditions(f.MustNot)
	if err != nil {
		return nil, fmt.Errorf("must_not: %w", err)
	}

	return &qdrant.Filter{
		Must:    must,
		Should:  should,
		MustNot: mustNot,
	}, nil
}

func toQdrantConditions(conditions []ports.FieldCondition) ([]*qdrant.Condition, error) {
	out := make([]*qdrant.Condition, 0, len(conditions))
	for idx, c := range conditions {
		cond, err := toQdrantCondition(c)
		if err != nil {
			return nil, fmt.Errorf("condition[%d] key=%q: %w", idx, c.Key, err)
		}
		out = append(out, cond)
	}
	return out, nil
}

func toQdrantCondition(c ports.FieldCondition) (*qdrant.Condition, error) {
	if c.Key == "" {
		return nil, fmt.Errorf("empty key")
	}

	switch c.Operator {
	case ports.MatchOperatorEqual:
		switch v := c.Value.(type) {
		case string:
			return qdrant.NewMatchKeyword(c.Key, v), nil
		case bool:
			return qdrant.NewMatchBool(c.Key, v), nil
		case int:
			return qdrant.NewMatchInt(c.Key, int64(v)), nil
		case int64:
			return qdrant.NewMatchInt(c.Key, v), nil
		case uint:
			if uint64(v) > math.MaxInt64 {
				return nil, fmt.Errorf("uint value out of int64 range: %d", v)
			}
			return qdrant.NewMatchInt(c.Key, int64(v)), nil
		case uint64:
			if v > math.MaxInt64 {
				return nil, fmt.Errorf("uint64 value out of int64 range: %d", v)
			}
			return qdrant.NewMatchInt(c.Key, int64(v)), nil
		default:
			return nil, fmt.Errorf("eq unsupported type: %T", c.Value)
		}

	case ports.MatchOperatorIn:
		switch v := c.Value.(type) {
		case []string:
			if len(v) == 0 {
				return nil, fmt.Errorf("in requires non-empty []string")
			}
			return qdrant.NewMatchKeywords(c.Key, v...), nil
		case []int:
			if len(v) == 0 {
				return nil, fmt.Errorf("in requires non-empty []int")
			}
			ints := make([]int64, 0, len(v))
			for _, item := range v {
				ints = append(ints, int64(item))
			}
			return qdrant.NewMatchInts(c.Key, ints...), nil
		case []int64:
			if len(v) == 0 {
				return nil, fmt.Errorf("in requires non-empty []int64")
			}
			return qdrant.NewMatchInts(c.Key, v...), nil
		case []uint64:
			if len(v) == 0 {
				return nil, fmt.Errorf("in requires non-empty []uint64")
			}
			ints := make([]int64, 0, len(v))
			for _, item := range v {
				if item > math.MaxInt64 {
					return nil, fmt.Errorf("uint64 item out of int64 range: %d", item)
				}
				ints = append(ints, int64(item))
			}
			return qdrant.NewMatchInts(c.Key, ints...), nil
		case []any:
			if len(v) == 0 {
				return nil, fmt.Errorf("in requires non-empty []any")
			}
			var strs []string
			var ints []int64
			for _, item := range v {
				switch x := item.(type) {
				case string:
					strs = append(strs, x)
				case int:
					ints = append(ints, int64(x))
				case int64:
					ints = append(ints, x)
				case uint64:
					if x > math.MaxInt64 {
						return nil, fmt.Errorf("uint64 item out of int64 range: %d", x)
					}
					ints = append(ints, int64(x))
				default:
					return nil, fmt.Errorf("in unsupported item type: %T", item)
				}
			}
			if len(strs) > 0 && len(ints) > 0 {
				return nil, fmt.Errorf("in mixed value types (string + integer) are not supported")
			}
			if len(strs) > 0 {
				return qdrant.NewMatchKeywords(c.Key, strs...), nil
			}
			if len(ints) > 0 {
				return qdrant.NewMatchInts(c.Key, ints...), nil
			}
			return nil, fmt.Errorf("in has no supported values")
		default:
			return nil, fmt.Errorf("in unsupported type: %T", c.Value)
		}
	default:
		return nil, fmt.Errorf("unsupported operator: %q", c.Operator)
	}
}

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
