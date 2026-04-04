package minio

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
)

type UploadLocalFileToMinIOUseCase struct {
	Bucket       string
	appLogger    util.Logger
	MinioStorage *minio.MinIOStorage
}

func NewUploadLocalFileToMinIOUseCase(bucket string, appLogger util.Logger, minioStorage *minio.MinIOStorage) *UploadLocalFileToMinIOUseCase {
	return &UploadLocalFileToMinIOUseCase{
		Bucket:       bucket,
		appLogger:    appLogger,
		MinioStorage: minioStorage,
	}
}

func (d *UploadLocalFileToMinIOUseCase) getReaderFile(filePath string) (io.ReadCloser, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.download_file.getReaderFile: Don't open file due to: ", err)
		return nil, 0, err
	}

	d.appLogger.Info("internal.application.use_cases.minio.download_file.getReaderFile: Open file successfully, path: ", filePath)

	info, err := file.Stat()
	if err != nil {
		file.Close()
		d.appLogger.Error("internal.application.use_cases.minio.download_file.getReaderFile: Don't stat file due to: ", err)
		return nil, 0, err
	}

	size := info.Size()

	d.appLogger.Info(
		"internal.application.use_cases.minio.download_file.getReaderFile: Get file info successfully, path: ",
		filePath,
		" size: ",
		size,
	)

	return file, size, nil
}

func getContentType(filename string) string {
	ext := strings.TrimSpace(filepath.Ext(filename))
	if ext != "" {
		return mime.TypeByExtension(ext)
	}
	return ""
}

func (d *UploadLocalFileToMinIOUseCase) sanitizeObjectKey(parsedURL *neturl.URL) string {
	objectKey := path.Base(parsedURL.Path)
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" || objectKey == "." || objectKey == "/" {
		return "uploaded-file"
	}
	return objectKey
}

func (d *UploadLocalFileToMinIOUseCase) buildTemporaryUploadFile(ctx context.Context, folderDownload string, rawURL string) (tempDir string, localFilePath string, objectKey string, err error) {
	if err = os.MkdirAll(folderDownload, 0755); err != nil {
		return "", "", "", err
	}

	parsedURL, err := neturl.Parse(rawURL)
	if err != nil {
		return "", "", "", err
	}
	objectKey = d.sanitizeObjectKey(parsedURL)

	tempDir, err = os.MkdirTemp(folderDownload, "minio-upload-*")
	if err != nil {
		return "", "", "", err
	}
	defer func() {
		if err == nil || tempDir == "" {
			return
		}
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			d.appLogger.Error("internal.application.use_cases.minio.download_file.buildTemporaryUploadFile: Don't clean up temporary folder on failure due to: ", removeErr)
		}
	}()

	localFilePath = filepath.Join(tempDir, objectKey)
	dstFile, err := os.Create(localFilePath)
	if err != nil {
		return tempDir, "", "", err
	}
	defer dstFile.Close()

	switch strings.ToLower(parsedURL.Scheme) {
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return tempDir, "", "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return tempDir, "", "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return tempDir, "", "", fmt.Errorf("download failed with status code %d", resp.StatusCode)
		}
		if _, err := io.Copy(dstFile, resp.Body); err != nil {
			return tempDir, "", "", err
		}
	default:
		localSource := rawURL
		if strings.EqualFold(parsedURL.Scheme, "file") {
			localSource = parsedURL.Path
		}
		srcFile, err := os.Open(localSource)
		if err != nil {
			return tempDir, "", "", err
		}
		defer srcFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return tempDir, "", "", err
		}
	}

	return tempDir, localFilePath, objectKey, nil
}

func (d *UploadLocalFileToMinIOUseCase) Execute(ctx context.Context, req *dtos.UploadFileMinioRequest) (io.ReadCloser, error) {
	if req == nil {
		err := fmt.Errorf("request is nil")
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Invalid request due to: ", err)
		return nil, err
	}

	folderDownload := strings.TrimSpace(req.FolderDownload)
	urlDownload := strings.TrimSpace(req.UrlDownload)
	if folderDownload == "" {
		err := fmt.Errorf("folder download is empty")
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Invalid folder download due to: ", err)
		return nil, err
	}
	if urlDownload == "" {
		err := fmt.Errorf("url download is empty")
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Invalid url download due to: ", err)
		return nil, err
	}

	tempDir, localFilePath, objectKey, err := d.buildTemporaryUploadFile(ctx, folderDownload, urlDownload)
	if err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Don't prepare temporary upload file due to: ", err)
		return nil, err
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Don't clean up temporary folder due to: ", removeErr)
			return
		}
		d.appLogger.Info("internal.application.use_cases.minio.download_file.Execute: Cleaned up temporary folder successfully, folder: ", tempDir)
	}()

	reader, size, err := d.getReaderFile(localFilePath)
	if err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Don't open temporary file for upload due to: ", err)
		return nil, err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Don't close temporary file reader due to: ", closeErr)
			return
		}
		d.appLogger.Info("internal.application.use_cases.minio.download_file.Execute: Closed temporary file reader successfully, path: ", localFilePath)
	}()

	input := ports.PutObjectInput{
		Bucket:      d.Bucket,
		ObjectKey:   objectKey,
		Reader:      reader,
		Size:        size,
		ContentType: getContentType(objectKey),
	}

	err = d.MinioStorage.EnsureBucket(ctx, d.Bucket)
	if err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Don't ensure bucket due to: ", err)
		return nil, err
	}
	_, err = d.MinioStorage.PutObject(ctx, input)
	if err != nil {
		d.appLogger.Error("internal.application.use_cases.minio.download_file.Execute: Don't put object to minio due to: ", err)
		return nil, err
	}
	d.appLogger.Info("internal.application.use_cases.minio.download_file.Execute: Put object to minio successfully, bucket: ", d.Bucket, " objectKey: ", objectKey)

	return nil, nil
}
