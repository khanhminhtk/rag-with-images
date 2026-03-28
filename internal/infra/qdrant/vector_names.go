package qdrant

import "rag_imagetotext_texttoimage/internal/application/ports"

const (
	vectorNameTextDense  = ports.VectorNameTextDense
	vectorNameImageDense = ports.VectorNameImageDense
	vectorNameBM25       = ports.VectorNameBM25

	vectorModelBM25 = "qdrant/bm25"
)
