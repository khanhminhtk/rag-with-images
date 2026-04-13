package trainingfile

import (
	"sort"
	"strings"
)

func (uc *trainingFileUseCase) resolveChunkingConfig() chunkingRuntimeConfig {
	cfg := chunkingRuntimeConfig{
		MinChunkChars:         uc.Config.FileTraining.MinChunkChars,
		MinChunkTokens:        uc.Config.FileTraining.MinChunkTokens,
		MaxChunkTokens:        uc.Config.FileTraining.MaxChunkTokens,
		SemanticSimThreshold:  uc.Config.FileTraining.SemanticSimilarityThreshold,
		ChunkOverlapSentences: uc.Config.FileTraining.ChunkOverlapSentences,
	}

	if cfg.MinChunkChars <= 0 {
		cfg.MinChunkChars = defaultMinChunkChars
	}
	if cfg.MinChunkTokens <= 0 {
		cfg.MinChunkTokens = defaultMinChunkTokens
	}
	if cfg.MaxChunkTokens <= 0 {
		cfg.MaxChunkTokens = defaultMaxChunkTokens
	}
	if cfg.SemanticSimThreshold <= 0 || cfg.SemanticSimThreshold >= 1 {
		cfg.SemanticSimThreshold = defaultSemanticThresh
	}
	if cfg.ChunkOverlapSentences < 0 {
		cfg.ChunkOverlapSentences = 0
	}
	if cfg.ChunkOverlapSentences == 0 {
		cfg.ChunkOverlapSentences = defaultChunkOverlap
	}
	return cfg
}

func countShortChunks(chunks []pipelineChunk, cfg chunkingRuntimeConfig) int {
	count := 0
	for _, chunk := range chunks {
		if isShortChunk(chunk, cfg) {
			count++
		}
	}
	return count
}

func percentInt(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return (float64(part) * 100.0) / float64(total)
}

func chunkTokenDistribution(chunks []pipelineChunk) (int, int, int) {
	if len(chunks) == 0 {
		return 0, 0, 0
	}

	tokens := make([]int, 0, len(chunks))
	maxTokens := 0
	for _, chunk := range chunks {
		t := countTextTokens(chunk.Text)
		tokens = append(tokens, t)
		if t > maxTokens {
			maxTokens = t
		}
	}
	sort.Ints(tokens)
	p50 := percentile(tokens, 50)
	p95 := percentile(tokens, 95)
	return p50, p95, maxTokens
}

func percentile(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := int((float64(p) / 100.0) * float64(len(sorted)-1))
	return sorted[idx]
}

func sampleChunkTexts(chunks []pipelineChunk, n int, maxLen int) ([]string, []string) {
	if len(chunks) == 0 || n <= 0 {
		return nil, nil
	}
	if maxLen <= 0 {
		maxLen = 200
	}

	headCount := n
	if headCount > len(chunks) {
		headCount = len(chunks)
	}
	tailCount := n
	if tailCount > len(chunks)-headCount {
		tailCount = len(chunks) - headCount
		if tailCount < 0 {
			tailCount = 0
		}
	}

	head := make([]string, 0, headCount)
	for i := 0; i < headCount; i++ {
		head = append(head, truncateForLog(chunks[i].Text, maxLen))
	}

	tail := make([]string, 0, tailCount)
	for i := len(chunks) - tailCount; i < len(chunks); i++ {
		if i < 0 || i < headCount {
			continue
		}
		tail = append(tail, truncateForLog(chunks[i].Text, maxLen))
	}
	return head, tail
}

func truncateForLog(text string, maxLen int) string {
	t := strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if len(t) <= maxLen {
		return t
	}
	if maxLen <= 3 {
		return t[:maxLen]
	}
	return t[:maxLen-3] + "..."
}
