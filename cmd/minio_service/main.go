package main

import (
	"rag_imagetotext_texttoimage/internal/bootstrap"
	"rag_imagetotext_texttoimage/internal/util"
)

func main() {
	app, err := bootstrap.NewMinioApp()
	if err != nil {
		util.Fatalf("failed to bootstrap minio runtime: %v", err)
	}
	defer app.Close()

	if err := app.Run(); err != nil {
		util.Fatalf("minio service stopped with error: %v", err)
	}
}
