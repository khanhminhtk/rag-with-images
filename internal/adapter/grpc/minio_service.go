package grpc

import (
	"context"
	"time"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	useCaseMinio "rag_imagetotext_texttoimage/internal/application/use_cases/minio"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type MinioService struct {
	pb.UnimplementedMinioServiceServer
	DeleteFileUseCase *useCaseMinio.DeleteFileInputUseCase
	PresignUseCase    *useCaseMinio.PresignGetObjectUseCase
	appLogger         util.Logger
}

func NewMinioService(deleteFileUseCase *useCaseMinio.DeleteFileInputUseCase, presignUseCase *useCaseMinio.PresignGetObjectUseCase, appLogger util.Logger) *MinioService {
	return &MinioService{
		DeleteFileUseCase: deleteFileUseCase,
		PresignUseCase:    presignUseCase,
		appLogger:         appLogger,
	}
}

func (m *MinioService) DeleteFile(ctx context.Context, req *pb.DeleteFileRequest) (*pb.DeleteFileResponse, error) {
	startedAt := time.Now()
	m.appLogger.Info("minio grpc DeleteFile started", "object_key", req.ObjectKey)

	err := m.DeleteFileUseCase.Execute(ctx, &dtos.DeleteFileMinioRequest{
		ObjectKey: req.ObjectKey,
	})
	if err != nil {
		m.appLogger.Error("delete file failed", err, "object_key", req.ObjectKey)
		return &pb.DeleteFileResponse{
			Success: false,
		}, err
	}
	m.appLogger.Info("minio grpc DeleteFile completed", "object_key", req.ObjectKey, "success", true, "latency_ms", time.Since(startedAt).Milliseconds())
	return &pb.DeleteFileResponse{
		Success: true,
	}, nil
}

func (m *MinioService) PresignUploadURL(ctx context.Context, req *pb.PresignUploadURLRequest) (*pb.PresignUploadURLResponse, error) {
	startedAt := time.Now()
	m.appLogger.Info("minio grpc PresignUploadURL started", "object_key", req.ObjectKey)

	url, err := m.PresignUseCase.Execute(ctx, &dtos.PresignGetObjectMinioRequest{
		ObjectKey: req.ObjectKey,
	})
	if err != nil {
		m.appLogger.Error("presign upload url failed", err, "object_key", req.ObjectKey)
		return &pb.PresignUploadURLResponse{
			Success: false,
			Url:     "",
		}, err
	}
	m.appLogger.Info("minio grpc PresignUploadURL completed", "object_key", req.ObjectKey, "success", true, "latency_ms", time.Since(startedAt).Milliseconds())
	return &pb.PresignUploadURLResponse{
		Success: true,
		Url:     url,
	}, nil
}
