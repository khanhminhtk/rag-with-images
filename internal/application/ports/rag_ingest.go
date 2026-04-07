package ports

import "context"

type RagVector struct {
	Name   string
	Vector []float32
}

type RagPoint struct {
	Vectors []RagVector
	Payload map[string]string
}

type RagPointWriter interface {
	InsertPoints(ctx context.Context, collectionName string, points []RagPoint) error
}
