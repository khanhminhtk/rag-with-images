package chat

import (
	"context"
	"sync"

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
	errCh := make(chan error, 3)
	defer wg.Done()

	var retrievalWg sync.WaitGroup

	if retrievalRequest.NewQuery != nil {
		retrievalWg.Add(1)
		go runRetrieval(
			&retrievalWg,
			errCh,
			func() (*pb.ResponseSearchPoint, error) {
				return c.RagServiceClient.SearchPoint(ctx, retrievalRequest.NewQuery)
			},
			func(resp *pb.ResponseSearchPoint) {
				if resp != nil && len(resp.Results) > 0 {
					results.NewQuery = resp.Results[0]
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
				return c.RagServiceClient.SearchPoint(ctx, retrievalRequest.CurrentQuery)
			},
			func(resp *pb.ResponseSearchPoint) {
				if resp != nil && len(resp.Results) > 0 {
					results.CurrentQuery = resp.Results[0]
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
				return c.RagServiceClient.SearchPoint(ctx, retrievalRequest.MultimodalQuery)
			},
			func(resp *pb.ResponseSearchPoint) {
				if resp != nil && len(resp.Results) > 0 {
					results.MultimodelQuery = resp.Results[0]
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
