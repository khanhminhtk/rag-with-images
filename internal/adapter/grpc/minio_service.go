package grpc

import (
	"context"

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
	err := m.DeleteFileUseCase.Execute(ctx, &dtos.DeleteFileMinioRequest{
		ObjectKey: req.ObjectKey,
	})
	if err != nil {
		m.appLogger.Error("internal.adapter.grpc.minio_service.DeleteFile: Error deleting file: %v", err)
		return &pb.DeleteFileResponse{
			Success: false,
		}, err
	}
	m.appLogger.Info("internal.adapter.grpc.minio_service.DeleteFile: File deleted successfully: %s", req.ObjectKey)
	return &pb.DeleteFileResponse{
		Success: true,
	}, nil
}

func (m *MinioService) PresignUploadURL(ctx context.Context, req *pb.PresignUploadURLRequest) (*pb.PresignUploadURLResponse, error) {
	url, err := m.PresignUseCase.Execute(ctx, &dtos.PresignGetObjectMinioRequest{
		ObjectKey: req.ObjectKey,
	})
	if err != nil {
		m.appLogger.Error("internal.adapter.grpc.minio_service.PresignUploadURL: Error generating presigned URL: %v", err)
		return &pb.PresignUploadURLResponse{
			Success: false,
			Url:     "",
		}, err
	}
	m.appLogger.Info("internal.adapter.grpc.minio_service.PresignUploadURL: Presigned URL generated successfully: %s", url)
	return &pb.PresignUploadURLResponse{
		Success: true,
		Url:     url,
	}, nil
}
