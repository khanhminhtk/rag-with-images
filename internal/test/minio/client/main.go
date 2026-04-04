package main


import (
	"log/slog"
	"rag_imagetotext_texttoimage/internal/infra/minio"
	"rag_imagetotext_texttoimage/internal/util"
)



func main() {
	appLogger, err := util.NewFileLogger(
		"/home/minhtk/code/rag_imtotext_texttoim/worktree/service-rag/logs/app.log",
		slog.LevelInfo,
	)

	if err != nil {
		panic(err)
	}
	config := minio.Config{
		Endpoint: "localhost:9000",
		AccessKey: "admin",
		SecretKey: "supersecretpassword",
		UseSSL: false,
		Region: "us-east-1",
	}

	minioClient, err := minio.NewMinioCleant(appLogger, config)
	if err != nil {
		appLogger.Error("internal.test.minio.client.main: Don't create minio client due to: ", err)
		return
	}
	_ = minioClient
	appLogger.Info("MinIO client created successfully!")

	
}
