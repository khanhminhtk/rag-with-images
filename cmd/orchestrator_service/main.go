package main

import (
	"rag_imagetotext_texttoimage/internal/bootstrap"
	"rag_imagetotext_texttoimage/internal/util"
)

func main() {
	app, err := bootstrap.NewOrchestratorApp()
	if err != nil {
		util.Fatalf("failed to bootstrap orchestrator runtime: %v", err)
	}
	defer app.Close()

	if err := app.Run(); err != nil {
		util.Fatalf("orchestrator service stopped with error: %v", err)
	}
}
