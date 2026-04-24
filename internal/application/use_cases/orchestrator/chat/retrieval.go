package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	pb "rag_imagetotext_texttoimage/proto"
)

func runRetrieval[Resp any](
	wg *sync.WaitGroup,
	errCh chan<- error,
	run func() (*Resp, error),
	assign func(*Resp),
) {
	defer wg.Done()
	resp, err := run()
	if err != nil {
		errCh <- err
		return
	}
	assign(resp)
}

func (c *ChatbotHandler) retrieval(
	ctx context.Context,
	wg *sync.WaitGroup,
	retrievalRequest RetrievalRequest,
) (RetrievalResult, error) {
	var results RetrievalResult
	var resultsMu sync.Mutex
	errCh := make(chan error, 3)
	defer wg.Done()

	var retrievalWg sync.WaitGroup

	if retrievalRequest.NewQuery != nil {
		retrievalWg.Add(1)
		go runRetrieval(
			&retrievalWg,
			errCh,
			func() (*pb.ResponseSearchPoint, error) {
				return c.searchPointWithDebug(ctx, "new_query", retrievalRequest.NewQuery)
			},
			func(resp *pb.ResponseSearchPoint) {
				if resp != nil && len(resp.Results) > 0 {
					resultsMu.Lock()
					defer resultsMu.Unlock()
					results.NewQuery = mergeResultsForContext(resp.Results)
				}
			},
		)
	}

	if retrievalRequest.CurrentQuery != nil {
		retrievalWg.Add(1)
		go runRetrieval(
			&retrievalWg,
			errCh,
			func() (*pb.ResponseSearchPoint, error) {
				return c.searchPointWithDebug(ctx, "current_query", retrievalRequest.CurrentQuery)
			},
			func(resp *pb.ResponseSearchPoint) {
				if resp != nil && len(resp.Results) > 0 {
					resultsMu.Lock()
					defer resultsMu.Unlock()
					results.CurrentQuery = mergeResultsForContext(resp.Results)
				}
			},
		)
	}

	if retrievalRequest.MultimodalQuery != nil {
		retrievalWg.Add(1)
		go runRetrieval(
			&retrievalWg,
			errCh,
			func() (*pb.ResponseSearchPoint, error) {
				return c.searchPointWithDebug(ctx, "multimodal_query", retrievalRequest.MultimodalQuery)
			},
			func(resp *pb.ResponseSearchPoint) {
				if resp != nil && len(resp.Results) > 0 {
					resultsMu.Lock()
					defer resultsMu.Unlock()
					results.MultimodelQuery = mergeResultsForContext(resp.Results)
				}
			},
		)
	}

	retrievalWg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

func (c *ChatbotHandler) searchPointWithDebug(
	ctx context.Context,
	retrievalType string,
	req *pb.SearchPointRequest,
) (*pb.ResponseSearchPoint, error) {
	startedAt := time.Now()
	if c != nil && c.appLogger != nil && req != nil {
		c.appLogger.Debug(
			"internal.application.use_cases.orchestrator.chat.retrieval search request",
			"retrieval_type", retrievalType,
			"collection_name", req.CollectionName,
			"vector_name", req.VectorName,
			"vector_dim", len(req.Vector),
			"limit", req.Limit,
			"with_payload", req.WithPayload,
		)
	}

	resp, err := c.RagServiceClient.SearchPoint(ctx, req)
	latency := time.Since(startedAt).Milliseconds()
	if err != nil {
		if c != nil && c.appLogger != nil {
			c.appLogger.Error(
				"internal.application.use_cases.orchestrator.chat.retrieval search failed",
				err,
				"retrieval_type", retrievalType,
				"collection_name", safeCollectionName(req),
				"vector_name", safeVectorName(req),
				"vector_dim", safeVectorDim(req),
				"latency_ms", latency,
			)
		}
		return nil, err
	}

	topScore := float32(-1)
	payloadPreview := ""
	resultCount := 0
	if resp != nil {
		resultCount = len(resp.Results)
		if resultCount > 0 && resp.Results[0] != nil {
			topScore = resp.Results[0].Score
			payloadPreview = previewPayload(resp.Results[0].Payload["text"])
		}
	}
	if c != nil && c.appLogger != nil {
		c.appLogger.Debug(
			"internal.application.use_cases.orchestrator.chat.retrieval search response",
			"retrieval_type", retrievalType,
			"collection_name", safeCollectionName(req),
			"vector_name", safeVectorName(req),
			"vector_dim", safeVectorDim(req),
			"result_count", resultCount,
			"top_score", topScore,
			"top_payload_preview", payloadPreview,
			"latency_ms", latency,
		)
	}
	return resp, nil
}

func safeCollectionName(req *pb.SearchPointRequest) string {
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.CollectionName)
}

func safeVectorName(req *pb.SearchPointRequest) string {
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.VectorName)
}

func safeVectorDim(req *pb.SearchPointRequest) int {
	if req == nil {
		return 0
	}
	return len(req.Vector)
}

func previewPayload(in string) string {
	trimmed := strings.TrimSpace(in)
	if trimmed == "" {
		return ""
	}
	const max = 220
	if len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max] + "..."
}

func mergeResultsForContext(results []*pb.SearchResultItem) *pb.SearchResultItem {
	if len(results) == 0 {
		return nil
	}
	base := results[0]
	if base == nil {
		return nil
	}

	payload := map[string]string{}
	for k, v := range base.Payload {
		payload[k] = v
	}

	parts := make([]string, 0, len(results))
	seen := map[string]struct{}{}
	for _, item := range results {
		if item == nil {
			continue
		}
		text := strings.TrimSpace(item.Payload["text"])
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	if len(parts) > 0 {
		payload["text"] = strings.Join(parts, "\n\n---\n\n")
	}

	return &pb.SearchResultItem{
		Id:      base.Id,
		Score:   base.Score,
		Payload: payload,
	}
}
