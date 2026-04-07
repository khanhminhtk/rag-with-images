package trainingfile

import (
	"context"

	pb "rag_imagetotext_texttoimage/proto"
)

func (uc *trainingFileUseCase) verifyVectorDBByDocID(ctx context.Context, collectionName, docID string) (bool, error) {
	conn, ragClient, err := uc.newRAGServiceClient(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	resp, err := ragClient.SearchPoint(ctx, &pb.SearchPointRequest{
		CollectionName: collectionName,
		VectorName:     "bm25",
		QueryText:      docID,
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
