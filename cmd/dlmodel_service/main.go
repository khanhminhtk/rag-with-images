package main

import (
	"rag_imagetotext_texttoimage/internal/bootstrap"
	"rag_imagetotext_texttoimage/internal/util"
)

func main() {
	app, err := bootstrap.NewDLModelApp()
	if err != nil {
		util.Fatalf("failed to bootstrap embedding runtime: %v", err)
	}
	defer app.Close()

	if err := app.Run(); err != nil {
		util.Fatalf("embedding service stopped with error: %v", err)
	}
}
