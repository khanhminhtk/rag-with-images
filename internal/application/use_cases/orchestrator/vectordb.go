package orchestrator

import (
	"context"
	"errors"
	"strings"

	pb "rag_imagetotext_texttoimage/proto"
)

type VectordbHandler struct {
	vectordbGrpcClient pb.RagServiceClient
}

func NewVectordbHandler(vectordbGrpcClient pb.RagServiceClient) *VectordbHandler {
	return &VectordbHandler{
		vectordbGrpcClient: vectordbGrpcClient,
	}
}

func (v *VectordbHandler) CreateCollection(ctx context.Context, req *pb.SchemaCollection) (bool, error) {
	if v == nil || v.vectordbGrpcClient == nil {
		return false, errors.New("vectordb grpc client is not configured")
	}
	if req == nil {
		return false, errors.New("create collection request is nil")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return false, errors.New("collection name is required")
	}

	resp, err := v.vectordbGrpcClient.CreateCollection(ctx, req)
	if err != nil {
		return false, err
	}
	if resp == nil {
		return false, errors.New("create collection response is nil")
	}
	return resp.Status, nil
}

func (v *VectordbHandler) DeleteCollection(ctx context.Context, collectionName string) (bool, error) {
	if v == nil || v.vectordbGrpcClient == nil {
		return false, errors.New("vectordb grpc client is not configured")
	}

	collectionName = strings.TrimSpace(collectionName)
	if collectionName == "" {
		return false, errors.New("collection name is required")
	}

	resp, err := v.vectordbGrpcClient.DeleteCollection(ctx, &pb.DeleteCollectionRequest{
		Name: collectionName,
	})
	if err != nil {
		return false, err
	}
	if resp == nil {
		return false, errors.New("delete collection response is nil")
	}
	return resp.Status, nil
}

func (v *VectordbHandler) DeletePointFilter(ctx context.Context, req *pb.DeletePointFilterRequest) (bool, error) {
	if v == nil || v.vectordbGrpcClient == nil {
		return false, errors.New("vectordb grpc client is not configured")
	}
	if req == nil {
		return false, errors.New("delete point filter request is nil")
	}
	req.CollectionName = strings.TrimSpace(req.CollectionName)
	if req.CollectionName == "" {
		return false, errors.New("collection name is required")
	}
	if req.Filter == nil {
		return false, errors.New("filter is required")
	}

	resp, err := v.vectordbGrpcClient.DeletePointFilter(ctx, req)
	if err != nil {
		return false, err
	}
	if resp == nil {
		return false, errors.New("delete point filter response is nil")
	}
	return resp.Status, nil
}
