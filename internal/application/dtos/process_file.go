package dtos

type DownFileTrainingRequest struct {
	UrlDownFile string `json:"url_down_file"`
	Uuid        string `json:"uuid"`
	PathSave    string `json:"path_save"`
}

type DownFileTrainingResult struct {
	Success  bool   `json:"success"`
	FilePath string `json:"file_path,omitempty"`
}

type AnalysisFileRequest struct {
	FilePath string `json:"file_path"`
	DistDir  string `json:"dist_dir"`
	Uuid     string `json:"uuid"`
	Dev      bool   `json:"dev"`
}

type AnalysisFileResult struct {
	Success bool `json:"success"`
}

type TrainingEmbeddingBatchTextRequest struct {
	Texts []string `json:"texts"`
}

type TrainingEmbeddingBatchImageRequest struct {
	ImagePaths []string `json:"image_paths"`
}

type TrainingEmbeddingBatchTextResult struct {
	Vectors   [][]float32 `json:"vectors"`
	Dimension int         `json:"dimension"`
}

type TrainingEmbeddingBatchImageResult struct {
	Vectors   [][]float32 `json:"vectors"`
	Dimension int         `json:"dimension"`
}

type UploadVectorDBRequest struct {
	CollectionName string                `json:"collection_name"`
	Points         []UploadVectorDBPoint `json:"points"`
	BatchSize      int                   `json:"batch_size,omitempty"`
}

type UploadVectorDBPoint struct {
	Vectors []UploadVectorDBVector `json:"vectors"`
	Payload map[string]string      `json:"payload"`
}

type UploadVectorDBVector struct {
	Name   string    `json:"name"`
	Vector []float32 `json:"vector"`
}

type UploadVectorDBResult struct {
	Success        bool  `json:"success"`
	InsertedPoints int   `json:"inserted_points"`
	SkippedPoints  int   `json:"skipped_points"`
	LatencyMs      int64 `json:"latency_ms"`
}

type ProcessAndIngestRequest struct {
	UUID           string `json:"uuid"`
	URLDownload    string `json:"url_download"`
	Lang           string `json:"lang,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type ProcessAndIngestResult struct {
	Success        bool   `json:"success"`
	UUID           string `json:"uuid"`
	DownloadPath   string `json:"download_path,omitempty"`
	ProcessDir     string `json:"process_dir,omitempty"`
	MarkdownPath   string `json:"markdown_path,omitempty"`
	UploadedFiles  int    `json:"uploaded_files"`
	InsertedPoints int    `json:"inserted_points"`
	SkippedPoints  int    `json:"skipped_points"`
	Verified       bool   `json:"verified"`
	LatencyMs      int64  `json:"latency_ms"`
}
