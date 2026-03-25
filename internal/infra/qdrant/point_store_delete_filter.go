package qdrant

import (
	"context"
	"fmt"

	"rag_imagetotext_texttoimage/internal/application/ports"

	"github.com/qdrant/go-client/qdrant"
)

func (p *PointStore) DeleteByFilter(ctx context.Context, collectionName string, filter ports.Filter) error {
	source := qdrantSource("PointStore.DeleteByFilter")
	p.appLogger.Debug("delete by filter started", "source", source, "collection", collectionName, "has_filter", !filter.IsEmpty())

	if collectionName == "" {
		err := fmt.Errorf("collection name is required")
		p.appLogger.Error("delete by filter validation failed", err, "source", source)
		return fmt.Errorf("%s: %w", source, err)
	}
	if filter.IsEmpty() {
		err := fmt.Errorf("filter is required")
		p.appLogger.Error("delete by filter validation failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: %w", source, err)
	}

	qFilter, err := toQdrantFilter(&filter)
	if err != nil {
		p.appLogger.Error("delete by filter conversion failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: invalid filter: %w", source, err)
	}

	wait := true
	_, err = p.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points:         qdrant.NewPointsSelectorFilter(qFilter),
	})
	if err != nil {
		p.appLogger.Error("delete by filter failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: qdrant delete by filter failed: %w", source, err)
	}

	p.appLogger.Info("delete by filter success", "source", source, "collection", collectionName)
	return nil
}
