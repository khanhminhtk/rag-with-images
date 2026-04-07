package ports

import (
	"context"

	"rag_imagetotext_texttoimage/internal/application/dtos"
)

type TrainingFileUseCase interface {
	Download(ctx context.Context, req *dtos.DownFileTrainingRequest) (dtos.DownFileTrainingResult, error)
	AnalysisFile(ctx context.Context, req *dtos.AnalysisFileRequest) (dtos.AnalysisFileResult, error)
	EmbeddingBatchTextTraining(ctx context.Context, req *dtos.TrainingEmbeddingBatchTextRequest) (dtos.TrainingEmbeddingBatchTextResult, error)
	EmbeddingBatchImageTraining(ctx context.Context, req *dtos.TrainingEmbeddingBatchImageRequest) (dtos.TrainingEmbeddingBatchImageResult, error)
	UploadVectorDB(ctx context.Context, req *dtos.UploadVectorDBRequest) (dtos.UploadVectorDBResult, error)
	ProcessAndIngest(ctx context.Context, req *dtos.ProcessAndIngestRequest) (dtos.ProcessAndIngestResult, error)
	DoSemanticChunking(ctx context.Context, emb [][]float32) error
}
