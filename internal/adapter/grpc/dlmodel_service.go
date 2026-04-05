package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type DLModelService struct {
	pb.UnimplementedDeepLearningServiceServer
	appLogger util.Logger
	infer     ports.Inference
}

func NewDLModelService(appLogger util.Logger, infer ports.Inference) *DLModelService {
	return &DLModelService{
		appLogger: appLogger,
		infer:     infer,
	}
}

func (s *DLModelService) EmbedText(ctx context.Context, req *pb.EmbedTextRequest) (*pb.EmbedTextResponse, error) {
	_ = ctx

	if req == nil {
		err := errors.New("request is nil")
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedText invalid request", err)
		return nil, err
	}

	request := &dtos.EmbedTextRequest{Text: strings.TrimSpace(req.Text)}
	if request.Text == "" {
		err := errors.New("text is required")
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedText invalid payload", err)
		return nil, err
	}

	embedding, err := s.infer.EmbedText(request.Text)
	if err != nil {
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedText inference failed", err)
		return nil, err
	}

	response := &dtos.EmbedTextResponse{
		Embedding: embedding,
		Dimension: len(embedding),
		Status:    true,
	}

	return &pb.EmbedTextResponse{
		Embedding: response.Embedding,
		Dimension: int32(response.Dimension),
		Status:    response.Status,
	}, nil
}

func (s *DLModelService) EmbedImage(ctx context.Context, req *pb.EmbedImageRequest) (*pb.EmbedImageResponse, error) {
	_ = ctx

	if req == nil {
		err := errors.New("request is nil")
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedImage invalid request", err)
		return nil, err
	}

	if len(req.Images) != 1 {
		err := fmt.Errorf("only single image is supported, got %d", len(req.Images))
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedImage batch input is not supported", err)
		return nil, err
	}

	request := &dtos.EmbedImageRequest{
		Pixels:   req.Images[0],
		Width:    int(req.Width),
		Height:   int(req.Height),
		Channels: int(req.Channels),
	}

	if len(request.Pixels) == 0 {
		err := errors.New("image payload is empty")
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedImage invalid payload", err)
		return nil, err
	}
	if request.Width <= 0 || request.Height <= 0 || request.Channels <= 0 {
		err := fmt.Errorf("invalid image shape width=%d height=%d channels=%d", request.Width, request.Height, request.Channels)
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedImage invalid shape", err)
		return nil, err
	}

	embedding, err := s.infer.EmbedImage(request.Pixels, request.Width, request.Height, request.Channels)
	if err != nil {
		s.appLogger.Error("internal.adapter.grpc.DLModelService.EmbedImage inference failed", err)
		return nil, err
	}

	response := &dtos.EmbedImageResponse{
		Embedding: embedding,
		Dimension: len(embedding),
		Status:    true,
	}

	return &pb.EmbedImageResponse{
		Embedding: response.Embedding,
		Dimension: int32(response.Dimension),
		Status:    response.Status,
	}, nil
}
