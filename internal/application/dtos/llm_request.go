package dtos

type ChatHistory struct {
	Role    string
	Content string
}

type LlmRequest struct {
	Temp            float32
	Prompt          string
	Model           string
	History         []ChatHistory
	ImageMode       bool
	StructureOutput map[string]any
}
