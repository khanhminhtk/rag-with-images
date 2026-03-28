package dto

import "rag_imagetotext_texttoimage/internal/application/ports"

type LlmRequest struct {
	Temp            float32
	Prompt          string
	Model           string
	History         []ports.ChatHistory
	ImageMode       bool
	StructureOutput map[string]any
}
