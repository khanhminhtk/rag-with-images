package trainingfile

import (
	"regexp"
	"time"
)

const (
	defaultPipelineTimeout = 180 * time.Second
	defaultSemanticThresh  = float32(0.85)
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
