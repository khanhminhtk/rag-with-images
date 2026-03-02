package llm

import (
	"context"
	"encoding/json"
	"os"

	"google.golang.org/genai"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type Gemini struct {
	Client    *genai.Client
	appLogger util.Logger
}

func NewGemini(config util.LLMConfig, ctx context.Context, appLogger util.Logger) (*Gemini, error) {
	cfg := &genai.ClientConfig{
		APIKey: config.ApiKey,
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		appLogger.Error("Failed to create Gemini client", err)
		return nil, err
	}

	appLogger.Info("Gemini client created successfully")

	return &Gemini{
		Client:    client,
		appLogger: appLogger,
	}, nil
}

func (G *Gemini) GenerateTextToText(
	model string,
	temp float32,
	prompt string,
	history []ports.ChatHistory,
	structureOutput map[string]any) (*ports.LLMResponse, error) {

	ctx := context.Background()

	config := &genai.GenerateContentConfig{
		Temperature: &temp,
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = structureOutput
	}

	var contents []*genai.Content

	if len(history) > 0 {
		for _, h := range history {
			historyContents := genai.Text(h.Content)
			if len(historyContents) > 0 {
				historyContents[0].Role = h.Role
				contents = append(contents, historyContents...)
			}
		}
	}

	promptContents := genai.Text(prompt)
	if len(promptContents) > 0 {
		promptContents[0].Role = "user"
		contents = append(contents, promptContents...)
	}

	result, err := G.Client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		G.appLogger.Error("[infra/llm/gemini/GenerateTextToText] error", err)
		return nil, err
	}

	G.appLogger.Info("[infra/llm/gemini/GenerateTextToText] successfully")

	response := &ports.LLMResponse{
		Text: result.Text(),
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		var jsonData map[string]any
		if err := json.Unmarshal([]byte(result.Text()), &jsonData); err != nil {
			G.appLogger.Error("[infra/llm/gemini/GenerateTextToText] failed to parse JSON", err)
		} else {
			response.JSON = jsonData
		}
	}

	G.appLogger.Debug("[infra/llm/gemini/GenerateTextToText] response", "text", response.Text)

	return response, nil
}

func (G *Gemini) GenerateTextToImage(
	model string,
	temp float32,
	imagePath string,
	prompt string,
	history []ports.ChatHistory,
	structureOutput map[string]any) (*ports.LLMResponse, error) {

	ctx := context.Background()

	config := &genai.GenerateContentConfig{
		Temperature: &temp,
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = structureOutput
	}

	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		G.appLogger.Error("[infra/llm/gemini/GenerateTextToImage] failed to read image", err)
		return nil, err
	}

	parts := []*genai.Part{
		genai.NewPartFromText(prompt),
		&genai.Part{
			InlineData: &genai.Blob{
				MIMEType: "image/jpeg",
				Data:     imgData,
			},
		},
	}

	var contents []*genai.Content

	if len(history) > 0 {
		for _, h := range history {
			historyContents := genai.Text(h.Content)
			if len(historyContents) > 0 {
				historyContents[0].Role = h.Role
				contents = append(contents, historyContents...)
			}
		}
	}

	contents = append(contents, genai.NewContentFromParts(parts, genai.RoleUser))

	result, err := G.Client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		G.appLogger.Error("[infra/llm/gemini/GenerateTextToImage] error", err)
		return nil, err
	}

	G.appLogger.Info("[infra/llm/gemini/GenerateTextToImage] successfully")

	response := &ports.LLMResponse{
		Text: result.Text(),
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		var jsonData map[string]any
		if err := json.Unmarshal([]byte(result.Text()), &jsonData); err != nil {
			G.appLogger.Error("[infra/llm/gemini/GenerateTextToImage] failed to parse JSON", err)
		} else {
			response.JSON = jsonData
		}
	}

	G.appLogger.Debug("[infra/llm/gemini/GenerateTextToImage] response", "text", response.Text)

	return response, nil
}
