package qdrant

import (
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"

	"github.com/qdrant/go-client/qdrant"
)

type PointStore struct {
	client    *qdrant.Client
	appLogger util.Logger
}

func NewPointStore(client *qdrant.Client, appLogger util.Logger) *PointStore {
	return &PointStore{client: client, appLogger: appLogger}
}

var _ ports.PointStore = (*PointStore)(nil)
