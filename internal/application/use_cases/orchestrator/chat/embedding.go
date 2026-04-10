package chat

import (
	"context"
	"strings"
	"sync"

	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)


func runEmbed[Resp any](
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

func (c *ChatbotHandler) fullEmbed(
	ctx context.Context,
	wg *sync.WaitGroup,
	executeQueries ExecuteQueries,
) (embeddingResults, error) {
	var results embeddingResults
	errCh := make(chan error, 1)
	defer wg.Done()

	var embedWg sync.WaitGroup
	embedWg.Add(1)
	go runEmbed(
		&embedWg,
		errCh,
		func() (*pb.EmbedTextResponse, error) {
			return c.ModelDLServiceClient.EmbedText(
				ctx,
				&pb.EmbedTextRequest{
					Text: executeQueries.CurrentQuery.TextDense,
				},
			)
		},
		func(resp *pb.EmbedTextResponse) {
			results.CurrentText = resp
		},
	)

	embedWg.Add(1)
	go runEmbed(
		&embedWg,
		errCh,
		func () (*pb.EmbedTextResponse, error) {
			return c.ModelDLServiceClient.EmbedText(
				ctx,
				&pb.EmbedTextRequest{
					Text: executeQueries.NewQuery.TextDense,
				},
			)
		},
		func(resp *pb.EmbedTextResponse) {
			results.NewText = resp
		},
	)

	
	if executeQueries.MultimodalQuery != nil && strings.TrimSpace(executeQueries.MultimodalQuery.ImageDense) != "" {
		embedWg.Add(1)
		pixels, width, height, channels, imgErr := util.LoadImagePixels(executeQueries.MultimodalQuery.ImageDense)
		if imgErr != nil {
			return embeddingResults{}, imgErr
		}
		go runEmbed(
			&embedWg,
			errCh,
			func() (*pb.EmbedImageResponse, error) {
				return c.ModelDLServiceClient.EmbedImage(
					ctx,
					&pb.EmbedImageRequest{
						Images:   [][]byte{pixels},
						Width:    int32(width),
						Height:   int32(height),
						Channels: int32(channels),
					},
				)
			},
			func(resp *pb.EmbedImageResponse) {
				results.Image = resp
			},
		)
	}

	embedWg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

