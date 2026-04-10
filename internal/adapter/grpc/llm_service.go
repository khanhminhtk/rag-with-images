package grpc

import (
	"context"
	"encoding/json"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type LLMService struct {
	pb.UnimplementedLlmServiceServer
	appLogger util.Logger
	llmClient ports.LLM
}

func parseChatHistory(pbHistory []*pb.ChatHistory) []dtos.ChatHistory {
	if len(pbHistory) == 0 {
		return nil
	}
	histories := make([]dtos.ChatHistory, 0, len(pbHistory))
	for _, his := range pbHistory {
		histories = append(histories, dtos.ChatHistory{
			Role:    his.Role,
			Content: his.Content,
		})
	}
	return histories
}

func toPortsChatHistory(history []dtos.ChatHistory) []ports.ChatHistory {
	if len(history) == 0 {
		return nil
	}
	out := make([]ports.ChatHistory, 0, len(history))
	for _, h := range history {
		out = append(out, ports.ChatHistory{
			Role:    h.Role,
			Content: h.Content,
		})
	}
	return out
}

func parseStructureOutput(pbStruct map[string]string) map[string]any {
	if len(pbStruct) == 0 {
		return nil
	}

	properties := make(map[string]any, len(pbStruct))
	required := make([]string, 0, len(pbStruct))

	for k, v := range pbStruct {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}

		value := strings.TrimSpace(v)
		if value == "" {
			value = "string"
		}

		// Allow advanced schema per-field via JSON value,
		// e.g. {"type":"array","items":{"type":"string"}}.
		var propertySchema map[string]any
		if strings.HasPrefix(value, "{") {
			if err := json.Unmarshal([]byte(value), &propertySchema); err == nil && len(propertySchema) > 0 {
				properties[key] = propertySchema
				required = append(required, key)
				continue
			}
		}

		typeName := strings.ToLower(value)
		switch typeName {
		case "string", "number", "integer", "boolean", "object", "array":
		default:
			typeName = "string"
		}

		properties[key] = map[string]any{
			"type": typeName,
		}
		required = append(required, key)
	}

	if len(properties) == 0 {
		return nil
	}

	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func parseLLMResponse(response *ports.LLMResponse) *pb.LLMResponse {
	pbResponse := &pb.LLMResponse{
		Text: response.Text,
	}

	if response.JSON != nil {
		pbResponse.Json = make(map[string]string)
		for k, v := range response.JSON {
			if strVal, ok := v.(string); ok {
				pbResponse.Json[k] = strVal
			}
		}
	}
	return pbResponse
}

func (S *LLMService) GenerateTextToText(ctx context.Context, req *pb.TextToTextRequest) (*pb.LLMResponse, error) {
	request := &dtos.LlmRequest{
		Temp:            req.Temperature,
		Prompt:          req.Prompt,
		Model:           req.Model,
		History:         parseChatHistory(req.History),
		ImageMode:       false,
		StructureOutput: parseStructureOutput(req.StructureOutput),
	}

	response, err := S.llmClient.GenerateTextToText(
		request.Model,
		request.Temp,
		request.Prompt,
		toPortsChatHistory(request.History),
		request.StructureOutput,
	)
	if err != nil {
		S.appLogger.Error("generate text to text failed", err, "model", request.Model)
		return nil, err
	}

	return parseLLMResponse(response), nil
}

func (S *LLMService) GenerateTextToImage(ctx context.Context, req *pb.TextToImageRequest) (*pb.LLMResponse, error) {
	request := &dtos.LlmRequest{
		Temp:            req.Temperature,
		Prompt:          req.Prompt,
		Model:           req.Model,
		History:         parseChatHistory(req.History),
		ImageMode:       true,
		StructureOutput: parseStructureOutput(req.StructureOutput),
	}

	response, err := S.llmClient.GenerateTextToImage(
		request.Model,
		request.Temp,
		req.ImagePath,
		request.Prompt,
		toPortsChatHistory(request.History),
		request.StructureOutput,
	)
	if err != nil {
		S.appLogger.Error("generate text to image failed", err, "model", request.Model)
		return nil, err
	}

	return parseLLMResponse(response), nil
}

func NewLLMService(appLogger util.Logger, llmClient ports.LLM) *LLMService {
	return &LLMService{
		appLogger: appLogger,
		llmClient: llmClient,
	}
}
