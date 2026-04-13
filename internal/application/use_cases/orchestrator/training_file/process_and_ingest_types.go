package trainingfile

import (
	"regexp"
	"time"
)

const (
	defaultPipelineTimeout = 180 * time.Second
	defaultSemanticThresh  = float32(0.85)
	defaultMinChunkChars   = 20
	defaultMinChunkTokens  = 8
	defaultMaxChunkTokens  = 160
	defaultChunkOverlap    = 1
)

var inlineImagePattern = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
var sentenceSplitPattern = regexp.MustCompile(`(?m)([^.!?\n]+[.!?]?)(\s+|$)`)

type lineTokenType string

const (
	lineTokenText  lineTokenType = "text"
	lineTokenImage lineTokenType = "image"
)

type lineToken struct {
	Type  lineTokenType
	Value string
}

type pipelineChunk struct {
	Text       string
	ImagePaths []string
}

type embeddingBatchResult struct {
	CorrelationID string      `json:"correlation_id"`
	Status        string      `json:"status"`
	Message       string      `json:"message"`
	Dimension     int         `json:"dimension"`
	Embeddings    [][]float32 `json:"embeddings"`
}

type chunkingRuntimeConfig struct {
	MinChunkChars         int
	MinChunkTokens        int
	MaxChunkTokens        int
	SemanticSimThreshold  float32
	ChunkOverlapSentences int
}

type chunkingStats struct {
	RawSentenceCount  int
	KeptSentenceCount int
	DroppedHeader     int
	DroppedNumeric    int
	DroppedNoise      int
	MergedShort       int
}
