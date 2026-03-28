package qdrant

import (
	"context"
	"fmt"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
)

func (p *PointStore) DeleteByIDs(ctx context.Context, collectionName string, ids []string) error {
	source := qdrantSource("PointStore.DeleteByIDs")
	p.appLogger.Debug("delete by ids started", "source", source, "collection", collectionName, "id_count", len(ids))

	if collectionName == "" {
		err := fmt.Errorf("collection name is required")
		p.appLogger.Error("delete by ids validation failed", err, "source", source)
		return fmt.Errorf("%s: %w", source, err)
	}
	if len(ids) == 0 {
		err := fmt.Errorf("ids are required")
		p.appLogger.Error("delete by ids validation failed", err, "source", source, "collection", collectionName)
		return fmt.Errorf("%s: %w", source, err)
	}

	for _, id := range ids {
		err := p.deleteByID(ctx, collectionName, id)
		if err != nil {
			p.appLogger.Error("delete by single id failed", err, "source", source, "collection", collectionName, "id", id)
			return fmt.Errorf("%s: delete id %q failed: %w", source, id, err)
		}
	}

	p.appLogger.Info("delete by ids success", "source", source, "collection", collectionName, "id_count", len(ids))
	return nil
}

func (p *PointStore) deleteByID(ctx context.Context, collectionName string, id string) error {
	source := qdrantSource("PointStore.deleteByID")
	wait := true

	_, err := p.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collectionName,
		Wait:           &wait,
		Points: qdrant.NewPointsSelector(
			toQdrantPointID(id),
		),
	})
	if err != nil {
		return fmt.Errorf("%s: qdrant delete failed: %w", source, err)
	}
	return nil
}

func toQdrantPointID(id string) *qdrant.PointId {
	if num, err := strconv.ParseUint(id, 10, 64); err == nil {
		return qdrant.NewIDNum(num)
	}
	return qdrant.NewID(id)
}
