package main

import (
	"context"
	"log/slog"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/infra/qdrant"
	"rag_imagetotext_texttoimage/internal/util"
)

func getclient(appLogger util.Logger) *qdrant.Client {
	config := qdrant.Config{
		Host: "localhost",
		Port: 6334,
	}

	client, err := qdrant.NewClient(config, appLogger)
	if err != nil {
		panic(err)
	}

	return client

}

func createCollection() {
	ctx := context.Background()

	appLogger, err := util.NewFileLogger(
		"/home/minhtk/code/rag_imtotext_texttoim/worktree/service-rag/logs/app.log",
		slog.LevelInfo,
	)

	if err != nil {
		panic(err)
	}

	client := getclient(appLogger)
	qdrantStore := qdrant.NewCollectionStore(
		client.Raw(),
		appLogger,
	)

	vectorTextconfig := ports.CollectionVectorConfig{
		Name:     "text_dense",
		Size:     4,
		Distance: ports.DistanceMetric("cosine"),
	}

	vectorImageconfig := ports.CollectionVectorConfig{
		Name:     "image_dense",
		Size:     4,
		Distance: ports.DistanceMetric("cosine"),
	}

	collectionCongif := ports.CollectionSchema{
		Name:              "test",
		Vectors:           []ports.CollectionVectorConfig{vectorTextconfig, vectorImageconfig},
		Shards:            uint32(1),
		ReplicationFactor: uint32(1),
		OnDiskPayload:     true,
		OptimizersMemmap:  true,
	}

	// qdrantStore.CreateCollection(ctx, collectionCongif)
	qdrantStore.EnsureCollection(ctx, collectionCongif)

}

func main() {
	createCollection()
}
