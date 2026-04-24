package trainingfile

import (
	"context"
	"fmt"
	"math"
)

type SemanticChunk struct {
	Texts     []string    `json:"text"`
	Embedding [][]float32 `json:"embedding"`
}

func consineSimilarity(vecA, vecB []float32) (float32, error) {
	if len(vecA) != len(vecB) {
		return 0, fmt.Errorf("vectors must have the same dimension")
	}

	var dotProduct float32
	var normA float32
	var normB float32

	for i := 0; i < len(vecA); i++ {
		dotProduct += vecA[i] * vecB[i]
		normA += vecA[i] * vecA[i]
		normB += vecB[i] * vecB[i]
	}

	if normA == 0 || normB == 0 {
		return 0, fmt.Errorf("cannot compute cosine similarity for zero-norm vectors")
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))), nil
}

func (uc *trainingFileUseCase) DoSemanticChunking(ctx context.Context, emb [][]float32, threshold float32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(emb) == 0 {
		return fmt.Errorf("embeddings is empty")
	}
	if len(emb[0]) == 0 {
		return fmt.Errorf("embedding at index 0 is empty")
	}
	if isZeroNormVector(emb[0]) {
		return fmt.Errorf("embedding at index 0 has zero norm")
	}

	dimension := len(emb[0])
	for i := 1; i < len(emb); i++ {
		if len(emb[i]) != dimension {
			return fmt.Errorf("embedding dimension mismatch at index %d: expected %d, got %d", i, dimension, len(emb[i]))
		}
		if isZeroNormVector(emb[i]) {
			return fmt.Errorf("embedding at index %d has zero norm", i)
		}
	}

	chunks := make([]SemanticChunk, 0, len(emb))

	currentChunk := SemanticChunk{
		Texts:     []string{fmt.Sprintf("segment_%d", 0)},
		Embedding: [][]float32{cloneFloat32Slice(emb[0])},
	}

	for i := 1; i < len(emb); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		similarity, err := consineSimilarity(emb[i-1], emb[i])
		if err != nil {
			return fmt.Errorf("compute cosine similarity at index %d: %w", i, err)
		}

		if similarity >= threshold {
			currentChunk.Texts = append(currentChunk.Texts, fmt.Sprintf("segment_%d", i))
			currentChunk.Embedding = append(currentChunk.Embedding, cloneFloat32Slice(emb[i]))
			continue
		}

		chunks = append(chunks, currentChunk)
		currentChunk = SemanticChunk{
			Texts:     []string{fmt.Sprintf("segment_%d", i)},
			Embedding: [][]float32{cloneFloat32Slice(emb[i])},
		}
	}
	chunks = append(chunks, currentChunk)

	if uc.logger != nil {
		uc.logger.Info(
			"internal.application.use_cases.orchestrator.training_file.DoSemanticChunking completed",
			"embedding_count", len(emb),
			"chunk_count", len(chunks),
			"dimension", dimension,
			"threshold", threshold,
		)
	}

	return nil
}

func cloneFloat32Slice(input []float32) []float32 {
	out := make([]float32, len(input))
	copy(out, input)
	return out
}
