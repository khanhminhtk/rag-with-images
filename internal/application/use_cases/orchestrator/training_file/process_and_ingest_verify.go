package trainingfile

import (
	"context"
	"strings"

	pb "rag_imagetotext_texttoimage/proto"
)

func (uc *trainingFileUseCase) verifyVectorDBByDocID(ctx context.Context, collectionName, docID, probeText string) (bool, error) {
	conn, ragClient, err := uc.newRAGServiceClient(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	// BM25 does not index doc_id itself, so we probe with actual chunk text while filtering by doc_id.
	queryText := strings.TrimSpace(probeText)
	if queryText == "" {
		queryText = docID
	}

	resp, err := ragClient.SearchPoint(ctx, &pb.SearchPointRequest{
		CollectionName: collectionName,
		VectorName:     "bm25",
		QueryText:      queryText,
		Limit:          1,
		WithPayload:    false,
		Filter: &pb.Filter{
			Must: []*pb.FieldCondition{
				{
					Key:      "doc_id",
					Operator: "eq",
					ScalarValue: &pb.FieldCondition_StringValue{
						StringValue: docID,
					},
				},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return resp != nil && len(resp.Results) > 0, nil
}
