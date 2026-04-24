package dtos

type UploadFileMinioRequest struct {
	FolderDownload string `json:"folderDownload" validate:"required"`
	UrlDownload       string `json:"urlDownload" validate:"required"`
}

type DeleteFileMinioRequest struct {
	ObjectKey string `json:"objectKey" validate:"required"`
}

type PresignGetObjectMinioRequest struct {
	ObjectKey string `json:"objectKey" validate:"required"`
}