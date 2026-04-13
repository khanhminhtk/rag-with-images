package chat

import (
	"context"
	"errors"
	"strings"

	portsOrchestrator "rag_imagetotext_texttoimage/internal/application/ports/orchestrator"
	pb "rag_imagetotext_texttoimage/proto"
)

func (c *ChatbotHandler) llmChat(
	ctx context.Context,
	prompt string,
	temperature float32,
	model string,
	query string,
	structOutput map[string]string,
	imagePath string,
	session_id string,
	stage string,
) (*pb.LLMResponse, error) {
	if c == nil || c.Session == nil {
		return nil, errors.New("session store is not configured")
	}

	sessionData, err := c.Session.GetSession(session_id)
	if err != nil {
		return nil, err
	}

	chatHistory := make([]portsOrchestrator.ChatMessage, 0)
	if sessionData.ChatHistory != nil {
		chatHistory, err = sessionData.ChatHistory.GetChatHistory()
		if err != nil {
			return nil, err
		}
	}

	history := make([]*pb.ChatHistory, 0, len(chatHistory))
	for _, h := range chatHistory {
		history = append(history, &pb.ChatHistory{
			Role:    h.Role,
			Content: h.Content,
		})
	}
	if c.appLogger != nil {
		c.appLogger.Info(
			"internal.application.use_cases.orchestrator.chat.llmChat request context",
			"stage", stage,
			"session_id", session_id,
			"history_count", len(history),
			"query_len", len(strings.TrimSpace(query)),
		)
	}

	fullPrompt := strings.TrimSpace(prompt + " " + query)

	if strings.TrimSpace(imagePath) != "" {
		resp, err := c.LLMServiceClient.GenerateTextToImage(
			ctx,
			&pb.TextToImageRequest{
				Model:           model,
				Temperature:     temperature,
				Prompt:          fullPrompt,
				History:         history,
				ImagePath:       imagePath,
				StructureOutput: structOutput,
			},
		)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	resp, err := c.LLMServiceClient.GenerateTextToText(
		ctx,
		&pb.TextToTextRequest{
			Model:           model,
			Temperature:     temperature,
			Prompt:          fullPrompt,
			History:         history,
			StructureOutput: structOutput,
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
