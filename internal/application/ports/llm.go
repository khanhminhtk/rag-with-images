package ports

type LLMResponse struct {
	Text string
	JSON map[string]any
}

type ChatHistory struct {
	Role string
	Content string
}

type LLM interface {
	GenerateTextToText(model string, temp float32, prompt string, history []ChatHistory, structureOutput map[string]any) (*LLMResponse, error)
	GenerateTextToImage(model string, temp float32, imagePath string, prompt string, history []ChatHistory, structureOutput map[string]any) (*LLMResponse, error)
}
