package main

import (
	"log/slog"
	"rag_imagetotext_texttoimage/internal/infra/qdrant"
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

	config := qdrant.Config{
		Host: "localhost",
		Port: 6334,
	}

	client, err := qdrant.NewClient(config, appLogger)
	if err != nil {
		panic(err)
	}

	client.Close()

}
