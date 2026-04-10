package chat

import (
	"context"
	"strings"

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
	uuid string,
) (*pb.LLMResponse, error) {
	sessionData, err := c.Session.GetSession(uuid)
	if err != nil {
		return nil, err
	}

	chatHistory, err := sessionData.ChatHistory.GetChatHistory()
	if err != nil {
		return nil, err
	}

	history := make([]*pb.ChatHistory, 0, len(chatHistory))
	for _, h := range chatHistory {
		history = append(history, &pb.ChatHistory{
			Role:    h.Role,
			Content: h.Content,
		})
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
