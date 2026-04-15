package usecases

import (
	"context"
	"errors"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
	"sort"
	"strings"
)

const (
	normEpsilon = 1e-8
)

type SearchWithVectorDB struct {
	appLogger        util.Logger
	VectorPointStore ports.PointStore
}

func NewSearchWithVectorDB(appLogger util.Logger, VectorPointStore ports.PointStore) *SearchWithVectorDB {
	return &SearchWithVectorDB{
		appLogger:        appLogger,
		VectorPointStore: VectorPointStore,
	}
}

type groupResult struct {
	items   [][]ports.SearchResult
	weights []float32
}

func (s *SearchWithVectorDB) Search(ctx context.Context, query ...ports.SearchQuery) ([]ports.SearchResult, error) {
	if len(query) < 1 {
		err := errors.New("no query provided")
		s.appLogger.Error("search failed", err)
		return nil, err
	}

	queries := s.expandQueries(query)
	group := groupResult{
		items:   make([][]ports.SearchResult, 0, len(queries)),
		weights: make([]float32, 0, len(queries)),
	}

	for _, q := range queries {
		results, err := s.searchAndNorm(ctx, q)
		if err != nil {
			return nil, err
		}
		group.items = append(group.items, results)
		group.weights = append(group.weights, defaultWeightForVector(q.VectorName))
	}

	merged := mergeWeighted(group)
	if len(merged) == 0 {
		return merged, nil
	}

	limit := query[0].Limit
	if limit > 0 && uint64(len(merged)) > limit {
		merged = merged[:limit]
	}

	return merged, nil
}

func (s *SearchWithVectorDB) expandQueries(query []ports.SearchQuery) []ports.SearchQuery {
	if len(query) == 0 {
		return query
	}

	var (
		hasTextDense bool
		hasBM25      bool
		baseQueryIdx = -1
	)

	for idx := range query {
		vectorName := strings.ToLower(strings.TrimSpace(query[idx].VectorName))
		switch vectorName {
		case ports.VectorNameTextDense:
			hasTextDense = true
			if baseQueryIdx == -1 && strings.TrimSpace(query[idx].QueryText) != "" {
				baseQueryIdx = idx
			}
		case ports.VectorNameBM25:
			hasBM25 = true
		}
	}

	if !hasTextDense || hasBM25 || baseQueryIdx == -1 {
		return query
	}

	qBM25 := query[baseQueryIdx]
	qBM25.VectorName = ports.VectorNameBM25
	qBM25.Vector = nil

	return append(query, qBM25)
}

func mergeWeighted(group groupResult) []ports.SearchResult {
	if len(group.items) == 0 {
		return nil
	}

	if len(group.weights) != len(group.items) {
		group.weights = make([]float32, len(group.items))
		for i := range group.weights {
			group.weights[i] = 1.0
		}
	}

	normalizeWeights(group.weights)
	merged := make(map[string]ports.SearchResult)

	for idx, results := range group.items {
		weight := group.weights[idx]

		for _, item := range results {
			if item.Point == nil {
				continue
			}

			existing, ok := merged[item.Point.ID]
			if !ok {
				item.Score = item.Score * weight
				merged[item.Point.ID] = item
				continue
			}
			existing.Score += item.Score * weight
			merged[item.Point.ID] = existing
		}
	}

	finalResult := make([]ports.SearchResult, 0, len(merged))
	for _, item := range merged {
		finalResult = append(finalResult, item)
	}

	sort.Slice(finalResult, func(i, j int) bool {
		return finalResult[i].Score > finalResult[j].Score
	})

	return finalResult
}

func normalizeWeights(weights []float32) {
	var sum float32
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum <= 0 {
		uniform := float32(1.0) / float32(len(weights))
		for i := range weights {
			weights[i] = uniform
		}
		return
	}
	for i := range weights {
		if weights[i] <= 0 {
			weights[i] = 0
			continue
		}
		weights[i] = weights[i] / sum
	}
}

func (s *SearchWithVectorDB) searchAndNorm(ctx context.Context, query ports.SearchQuery) ([]ports.SearchResult, error) {
	resultSearch, err := s.searchOnlyQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	normalizationMode := normFullScore(&resultSearch)
	s.appLogger.Info(
		"search normalization mode",
		"vector_name", query.VectorName,
		"result_count", len(resultSearch),
		"normalization_mode", normalizationMode,
		"top_score_after_norm", topScore(resultSearch),
	)
	return resultSearch, nil
}

func normFullScore(resultSearch *[]ports.SearchResult) string {
	if resultSearch == nil || len(*resultSearch) == 0 {
		return "empty"
	}
	if len(*resultSearch) < 2 {
		// Keep raw score when there is only one candidate.
		return "raw_single"
	}

	scores := extractScores(resultSearch)
	minScore, maxScore := getMinMax(scores)
	if (maxScore - minScore) <= normEpsilon {
		// Keep raw scores for flat distributions to avoid forcing all candidates to 0.5.
		return "raw_flat"
	}
	for idx, score := range scores {
		(*resultSearch)[idx].Score = normScoreCandidate(score, minScore, maxScore, normEpsilon)
	}
	return "minmax"
}

func getMinMax(scores []float32) (float32, float32) {
	minVal := scores[0]
	maxVal := scores[0]

	for _, v := range scores[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	return minVal, maxVal
}

func (s *SearchWithVectorDB) searchOnlyQuery(ctx context.Context, query ports.SearchQuery) ([]ports.SearchResult, error) {
	result, err := s.VectorPointStore.Search(ctx, query)
	if err != nil {
		s.appLogger.Error("search failed", err, "vector_name", query.VectorName)
		return nil, err
	}
	s.appLogger.Info("search success", "vector_name", query.VectorName, "result_count", len(result))
	return result, nil
}

func extractScores(results *[]ports.SearchResult) []float32 {
	scores := make([]float32, 0, len(*results))
	for _, result := range *results {
		scores = append(scores, result.Score)
	}
	return scores
}

func normScoreCandidate(s float32, sMin float32, sMax float32, eps float32) float32 {
	// Qdrant dense/bm25 scores are relevance scores: larger is better.
	// Min-max normalization keeps ordering and handles BM25 values > 1 safely.
	return (s - sMin) / (sMax - sMin + eps)
}

func defaultWeightForVector(vectorName string) float32 {
	switch strings.TrimSpace(strings.ToLower(vectorName)) {
	case ports.VectorNameTextDense:
		return 0.5
	case ports.VectorNameImageDense:
		return 0.2
	case ports.VectorNameBM25:
		return 0.2
	default:
		return 1.0
	}
}

func topScore(results []ports.SearchResult) float32 {
	if len(results) == 0 {
		return -1
	}
	return results[0].Score
}
