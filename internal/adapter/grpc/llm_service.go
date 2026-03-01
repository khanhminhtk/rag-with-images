package grpc

import (
	"context"

	"rag_imagetotext_texttoimage/internal/application/dto"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type LLMService struct {
	pb.UnimplementedLlmServiceServer
	appLogger util.Logger
	llmClient ports.LLM
}

func parseChatHistory(pbHistory []*pb.ChatHistory) []ports.ChatHistory {
	if len(pbHistory) == 0 {
		return nil
	}
	histories := make([]ports.ChatHistory, 0, len(pbHistory))
	for _, his := range pbHistory {
		histories = append(histories, ports.ChatHistory{
			Role:    his.Role,
			Content: his.Content,
		})
	}
	return histories
}

func parseStructureOutput(pbStruct map[string]string) map[string]any {
	if len(pbStruct) == 0 {
		return nil
	}
	structureOutput := make(map[string]any, len(pbStruct))
	for k, v := range pbStruct {
		structureOutput[k] = v
	}
	return structureOutput
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
	request := &dto.LlmRequest{
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
		request.History,
		request.StructureOutput,
	)
	if err != nil {
		S.appLogger.Error("Failed to generate text to text: ", err)
		return nil, err
	}

	return parseLLMResponse(response), nil
}

func (S *LLMService) GenerateTextToImage(ctx context.Context, req *pb.TextToImageRequest) (*pb.LLMResponse, error) {
	request := &dto.LlmRequest{
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
		request.History,
		request.StructureOutput,
	)
	if err != nil {
		S.appLogger.Error("Failed to generate text to image: ", err)
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
