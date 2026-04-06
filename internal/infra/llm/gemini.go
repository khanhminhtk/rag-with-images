package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"google.golang.org/genai"

	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type Gemini struct {
	Client       *genai.Client
	defaultModel string
	defaultTemp  float32
	appLogger    util.Logger
}

func NewGemini(config util.LLMSettings, ctx context.Context, appLogger util.Logger) (*Gemini, error) {
	apiKey := strings.TrimSpace(config.ApiKey)
	if apiKey == "" {
		return nil, errors.New("gemini api key is empty")
	}

	defaultModel := strings.TrimSpace(config.Model)
	if defaultModel == "" {
		return nil, errors.New("llm model is empty")
	}

	defaultTemp := config.Temp
	if defaultTemp <= 0 {
		return nil, errors.New("llm temperature must be greater than zero")
	}

	cfg := &genai.ClientConfig{
		APIKey: apiKey,
	}

	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		appLogger.Error("create gemini client failed", err)
		return nil, err
	}

	appLogger.Info("create gemini client success", "default_model", defaultModel, "default_temp", defaultTemp)

	return &Gemini{
		Client:       client,
		defaultModel: defaultModel,
		defaultTemp:  defaultTemp,
		appLogger:    appLogger,
	}, nil
}

func (G *Gemini) GenerateTextToText(
	model string,
	temp float32,
	prompt string,
	history []ports.ChatHistory,
	structureOutput map[string]any) (*ports.LLMResponse, error) {

	ctx := context.Background()
	model = strings.TrimSpace(model)
	if model == "" {
		model = G.defaultModel
	}
	if temp <= 0 {
		temp = G.defaultTemp
	}

	config := &genai.GenerateContentConfig{
		Temperature: &temp,
	}

	if len(structureOutput) > 0 {
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
		G.appLogger.Error("generate text to text failed", err, "model", model)
		return nil, err
	}

	G.appLogger.Info("generate text to text success", "model", model)

	response := &ports.LLMResponse{
		Text: result.Text(),
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		var jsonData map[string]any
		if err := json.Unmarshal([]byte(result.Text()), &jsonData); err != nil {
			G.appLogger.Error("generate text to text parse json failed", err, "model", model)
		} else {
			response.JSON = jsonData
		}
	}

	G.appLogger.Debug("generate text to text response", "model", model, "text", response.Text)

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
	model = strings.TrimSpace(model)
	if model == "" {
		model = G.defaultModel
	}
	if temp <= 0 {
		temp = G.defaultTemp
	}

	config := &genai.GenerateContentConfig{
		Temperature: &temp,
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		config.ResponseMIMEType = "application/json"
		config.ResponseJsonSchema = structureOutput
	}

	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		G.appLogger.Error("generate text to image read image failed", err, "image_path", imagePath)
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
		G.appLogger.Error("generate text to image failed", err, "model", model)
		return nil, err
	}

	G.appLogger.Info("generate text to image success", "model", model)

	response := &ports.LLMResponse{
		Text: result.Text(),
	}

	if structureOutput != nil && len(structureOutput) > 0 {
		var jsonData map[string]any
		if err := json.Unmarshal([]byte(result.Text()), &jsonData); err != nil {
			G.appLogger.Error("generate text to image parse json failed", err, "model", model)
		} else {
			response.JSON = jsonData
		}
	}

	G.appLogger.Debug("generate text to image response", "model", model, "text", response.Text)

	return response, nil
}
