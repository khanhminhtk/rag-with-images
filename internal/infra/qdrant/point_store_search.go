package qdrant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	domain "rag_imagetotext_texttoimage/internal/domain/entity_objects"

	"github.com/qdrant/go-client/qdrant"
)

type searchMode int

const (
	searchModeDense searchMode = iota
	searchModeLexical
)

func (p *PointStore) Search(ctx context.Context, query ports.SearchQuery) ([]ports.SearchResult, error) {
	source := qdrantSource("PointStore.Search")
	mode, err := detectSearchMode(query)
	if err != nil {
		p.appLogger.Error("search mode detection failed", err, "source", source, "collection", query.CollectionName, "vector_name", query.VectorName)
		return nil, fmt.Errorf("%s: detect search mode failed: %w", source, err)
	}
	results, err := p.searchByMode(ctx, query, mode)
	if err != nil {
		p.appLogger.Error("search execution failed", err, "source", source, "collection", query.CollectionName, "vector_name", query.VectorName, "mode", mode.String())
		return nil, fmt.Errorf("%s: search by mode failed: %w", source, err)
	}
	return results, nil
}

func detectSearchMode(query ports.SearchQuery) (searchMode, error) {
	source := qdrantSource("detectSearchMode")
	vectorName := strings.TrimSpace(strings.ToLower(query.VectorName))
	switch vectorName {
	case vectorNameBM25:
		return searchModeLexical, nil
	case vectorNameTextDense, vectorNameImageDense:
		return searchModeDense, nil
	case "":
		// fall through to backward-compat data-based detection
	default:
		if len(query.Vector) > 0 {
			// Allow custom dense vector names while still honoring explicit vector_name.
			return searchModeDense, nil
		}
		return 0, fmt.Errorf("%s: unsupported vector name %q", source, query.VectorName)
	}

	if len(query.Vector) > 0 {
		return searchModeDense, nil
	}
	if strings.TrimSpace(query.QueryText) != "" {
		return searchModeLexical, nil
	}
	return 0, fmt.Errorf("%s: either query vector or query text is required", source)
}

func (p *PointStore) searchByMode(ctx context.Context, query ports.SearchQuery, mode searchMode) ([]ports.SearchResult, error) {
	source := qdrantSource("PointStore.searchByMode")
	if query.CollectionName == "" {
		return nil, fmt.Errorf("%s: collection name is required", source)
	}
	queryText := strings.TrimSpace(query.QueryText)
	if mode == searchModeLexical {
		if queryText == "" {
			return nil, fmt.Errorf("%s: query text is required for lexical search", source)
		}
	} else if len(query.Vector) == 0 {
		return nil, fmt.Errorf("%s: query vector is required", source)
	}
	if query.Limit == 0 {
		return nil, fmt.Errorf("%s: limit must be greater than 0", source)
	}

	startedAt := time.Now()
	logFields := buildSearchLogFields(query, mode, queryText, source)
	p.appLogger.Debug("qdrant search started", logFields...)

	var reqQuery *qdrant.Query
	if mode == searchModeLexical {
		reqQuery = qdrant.NewQueryNearest(
			qdrant.NewVectorInputDocument(&qdrant.Document{
				Model: vectorModelBM25,
				Text:  queryText,
			}),
		)
	} else {
		reqQuery = qdrant.NewQuery(query.Vector...)
	}

	req := &qdrant.QueryPoints{
		CollectionName: query.CollectionName,
		Query:          reqQuery,
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
			return nil, fmt.Errorf("%s: invalid filter: %w", source, err)
		}
		req.Filter = filter
	}

	points, err := p.client.Query(ctx, req)
	if err != nil {
		p.appLogger.Error(
			"qdrant search failed",
			err,
			append(logFields, "duration_ms", time.Since(startedAt).Milliseconds())...,
		)
		return nil, fmt.Errorf("%s: qdrant query failed: %w", source, err)
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

	p.appLogger.Info(
		"qdrant search success",
		append(
			logFields,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"result_count", len(results),
		)...,
	)

	return results, nil
}

func buildSearchLogFields(query ports.SearchQuery, mode searchMode, queryText string, source string) []any {
	fields := []any{
		"component", "qdrant_point_store",
		"source", source,
		"operation", "search",
		"mode", mode.String(),
		"collection", query.CollectionName,
		"vector_name", query.VectorName,
		"limit", query.Limit,
		"with_payload", query.WithPayload,
		"has_filter", query.Filter != nil && !query.Filter.IsEmpty(),
		"vector_dim", len(query.Vector),
		"query_text_len", len(queryText),
	}
	if query.ScoreThreshold != nil {
		fields = append(fields, "score_threshold", *query.ScoreThreshold)
	}
	return fields
}

func (m searchMode) String() string {
	switch m {
	case searchModeDense:
		return "dense"
	case searchModeLexical:
		return "lexical"
	default:
		return "unknown"
	}
}
