package qdrant

import (
	"context"
	"fmt"
	"time"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"

	"github.com/qdrant/go-client/qdrant"
)

type CollectionStore struct {
	client    *qdrant.Client
	appLogger util.Logger
}

func NewCollectionStore(client *qdrant.Client, appLogger util.Logger) *CollectionStore {
	return &CollectionStore{
		client:    client,
		appLogger: appLogger,
	}
}

var _ ports.CollectionStore = (*CollectionStore)(nil)

func toQdrantDistance(d ports.DistanceMetric) qdrant.Distance {
	switch d {
	case ports.DistanceCosine:
		return qdrant.Distance_Cosine
	case ports.DistanceEuclid:
		return qdrant.Distance_Euclid
	case ports.DistanceDot:
		return qdrant.Distance_Dot
	case ports.DistanceManhattan:
		return qdrant.Distance_Manhattan
	default:
		return qdrant.Distance_Cosine
	}
}

func (c *CollectionStore) CreateCollection(ctx context.Context, schema ports.CollectionSchema) error {
	source := qdrantSource("CollectionStore.CreateCollection")
	c.appLogger.Debug(
		"create collection started",
		"source", source,
		"collection", schema.Name,
		"vector_count", len(schema.Vectors),
	)

	if len(schema.Vectors) == 0 {
		err := fmt.Errorf("collection schema must contain at least one vector config")
		c.appLogger.Error("create collection validation failed", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: %w", source, err)
	}

	req := &qdrant.CreateCollection{
		CollectionName:      schema.Name,
		OnDiskPayload:       &schema.OnDiskPayload,
		SparseVectorsConfig: buildBM25SparseVectorsConfig(schema.OptimizersMemmap),
	}

	if schema.Shards > 0 {
		shards := uint32(schema.Shards)
		req.ShardNumber = &shards
	}

	if schema.ReplicationFactor > 0 {
		rf := uint32(schema.ReplicationFactor)
		req.ReplicationFactor = &rf
	}

	if len(schema.Vectors) == 1 {
		v := schema.Vectors[0]
		onDisk := schema.OptimizersMemmap
		req.VectorsConfig = qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     v.Size,
			Distance: toQdrantDistance(v.Distance),
			OnDisk:   &onDisk,
		})
	} else {
		paramsMap := make(map[string]*qdrant.VectorParams, len(schema.Vectors))
		for _, v := range schema.Vectors {
			onDisk := schema.OptimizersMemmap

			paramsMap[v.Name] = &qdrant.VectorParams{
				Size:     v.Size,
				Distance: toQdrantDistance(v.Distance),
				OnDisk:   &onDisk,
			}
		}
		req.VectorsConfig = qdrant.NewVectorsConfigMap(paramsMap)
	}

	if schema.OptimizersMemmap {
		memmapThreshold := uint64(1)
		req.OptimizersConfig = &qdrant.OptimizersConfigDiff{
			MemmapThreshold: &memmapThreshold,
		}
	}

	if err := c.client.CreateCollection(ctx, req); err != nil {
		c.appLogger.Error("create collection failed", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: create collection failed: %w", source, err)
	}

	c.appLogger.Info("create collection success", "source", source, "collection", schema.Name, "vector_count", len(schema.Vectors))
	return nil
}

func buildBM25SparseVectorsConfig(onDisk bool) *qdrant.SparseVectorConfig {
	modifier := qdrant.Modifier_Idf
	return qdrant.NewSparseVectorsConfig(map[string]*qdrant.SparseVectorParams{
		vectorNameBM25: {
			Modifier: &modifier,
			Index: &qdrant.SparseIndexConfig{
				OnDisk: &onDisk,
			},
		},
	})
}

func (c *CollectionStore) CollectionExists(ctx context.Context, collectionName string) (bool, error) {
	source := qdrantSource("CollectionStore.CollectionExists")
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	exists, err := c.client.CollectionExists(timeoutCtx, collectionName)
	if err != nil {
		c.appLogger.Error("collection exists check failed", err, "source", source, "collection", collectionName)
		return false, fmt.Errorf("%s: collection exists check failed: %w", source, err)
	}

	c.appLogger.Info("collection exists check success", "source", source, "collection", collectionName, "exists", exists)
	return exists, nil
}

func (c *CollectionStore) EnsureCollection(ctx context.Context, schema ports.CollectionSchema) error {
	source := qdrantSource("CollectionStore.EnsureCollection")
	exists, err := c.CollectionExists(ctx, schema.Name)
	if err != nil {
		c.appLogger.Error("ensure collection failed to check existence", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: collection exists check failed: %w", source, err)
	}
	if exists {
		c.appLogger.Info("ensure collection skipped, already exists", "source", source, "collection", schema.Name)
		return nil
	}

	c.appLogger.Info("ensure collection creating", "source", source, "collection", schema.Name)
	if err := c.CreateCollection(ctx, schema); err != nil {
		c.appLogger.Error("ensure collection create failed", err, "source", source, "collection", schema.Name)
		return fmt.Errorf("%s: create collection failed: %w", source, err)
	}
	return nil
}

func (c *CollectionStore) DeleteCollection(ctx context.Context, collectionName string) error {
	source := qdrantSource("CollectionStore.DeleteCollection")
	if err := c.client.DeleteCollection(ctx, collectionName); err != nil {
		c.appLogger.Error("delete collection failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: delete collection failed: %w", source, err)
	}

	c.appLogger.Info("delete collection success", "source", source, "collection", collectionName)
	return nil
}
