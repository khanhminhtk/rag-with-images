package chat

import (
	"context"
	"errors"
	"strings"
	"sync"

	"rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
	portsOrchestrator"rag_imagetotext_texttoimage/internal/application/ports/orchestrator"
	"rag_imagetotext_texttoimage/internal/util"
	pb "rag_imagetotext_texttoimage/proto"
)

type ChatbotHandler struct {
	Session              *orchestrator.InMemorySessionStore
	appLogger            util.Logger
	Config               util.Config
	RagServiceClient     pb.RagServiceClient
	ModelDLServiceClient pb.DeepLearningServiceClient
	LLMServiceClient     pb.LlmServiceClient
	PromptPreprocessing  string
	PromptPostprocessing string
	PromptAnswer         string
}

func NewChatbotHandler(
	session *orchestrator.InMemorySessionStore,
	config util.Config,
	ragClient pb.RagServiceClient,
	modelDLClient pb.DeepLearningServiceClient,
	llmClient pb.LlmServiceClient,
	PromptPreprocessing string,
	PromptPostprocessing string,
	PromptAnswer string,
) *ChatbotHandler {
	return &ChatbotHandler{
		Session:              session,
		Config:               config,
		RagServiceClient:     ragClient,
		ModelDLServiceClient: modelDLClient,
		LLMServiceClient:     llmClient,
		PromptPreprocessing:  PromptPreprocessing,
		PromptPostprocessing: PromptPostprocessing,
		PromptAnswer:         PromptAnswer,
	}
}

type QueryPayload struct {
	TextDense  string `json:"Text_Dense,omitempty"`
	ImageDense string `json:"Image_Dense,omitempty"`
}

type ExecuteQueries struct {
	NewQuery        QueryPayload  `json:"NewQuery"`
	CurrentQuery    QueryPayload  `json:"CurrentQuery"`
	MultimodalQuery *QueryPayload `json:"MultimodalQuery,omitempty"`
}

type embeddingResults struct {
	NewText     *pb.EmbedTextResponse
	CurrentText *pb.EmbedTextResponse
	Image       *pb.EmbedImageResponse
}

type RetrievalRequest struct {
	NewQuery        *pb.SearchPointRequest
	CurrentQuery    *pb.SearchPointRequest
	MultimodalQuery *pb.SearchPointRequest
}

type RetrievalResult struct {
	NewQuery        *pb.SearchResultItem
	CurrentQuery    *pb.SearchResultItem
	MultimodelQuery *pb.SearchResultItem
}

func (c *ChatbotHandler) Execute(
	ctx context.Context,
	query string,
	imagePath string,
	uuid string,
) (string, error) {
	exist, err := c.Session.SessionExists(uuid)
	if err != nil {
		if c.appLogger != nil {
			c.appLogger.Error("internal.application.use_cases.orchestrator.chat.SessionExists failed", err)
		}
		return "", err
	}
	if !exist {
		if err := c.Session.CreateSession(uuid); err != nil {
			if c.appLogger != nil {
				c.appLogger.Error("internal.application.use_cases.orchestrator.chat.CreateSession failed", err)
			}
			return "", err
		}
	}

	responsePreprocess, err := c.llmChat(
		ctx,
		c.PromptPreprocessing,
		c.Config.OrchestratorService.PreProcessing.Temperature,
		c.Config.OrchestratorService.PreProcessing.Model,
		query,
		c.Config.OrchestratorService.PreProcessing.StructOutput,
		imagePath,
		uuid,
	)
	if err != nil {
		return "", err
	}

	var executeQueries ExecuteQueries
	executeQueries.CurrentQuery.TextDense = strings.TrimSpace(responsePreprocess.Json["CurrentQuery"])
	executeQueries.NewQuery.TextDense = strings.TrimSpace(responsePreprocess.Json["NewQuery"])
	if strings.TrimSpace(imagePath) != "" {
		executeQueries.MultimodalQuery = &QueryPayload{ImageDense: imagePath}
	}

	var wg sync.WaitGroup
	var embedResults embeddingResults
	var embedErr error
	wg.Add(1)
	go func() {
		embedResults, embedErr = c.fullEmbed(
			ctx,
			&wg,
			executeQueries,
		)
	}()
	wg.Wait()
	if embedErr != nil {
		return "", embedErr
	}

	collectionName := strings.TrimSpace(responsePreprocess.Json["CollectionName"])
	if collectionName == "" {
		collectionName = strings.TrimSpace(responsePreprocess.Json["collection_name"])
	}
	if collectionName == "" {
		return "", errors.New("collection name is required")
	}

	retrievalReq := RetrievalRequest{
		NewQuery: &pb.SearchPointRequest{
			CollectionName: collectionName,
			VectorName:     "text_dense",
			Vector:         embedResults.NewText.Embedding,
			Limit:          1,
			WithPayload:    true,
		},
		CurrentQuery: &pb.SearchPointRequest{
			CollectionName: collectionName,
			VectorName:     "text_dense",
			Vector:         embedResults.CurrentText.Embedding,
			Limit:          1,
			WithPayload:    true,
		},
	}
	if embedResults.Image != nil {
		retrievalReq.MultimodalQuery = &pb.SearchPointRequest{
			CollectionName: collectionName,
			VectorName:     "image_dense",
			Vector:         embedResults.Image.Embedding,
			Limit:          1,
			WithPayload:    true,
		}
	}

	var retrievalResults RetrievalResult
	var retrievalErr error
	wg.Add(1)
	go func() {
		retrievalResults, retrievalErr = c.retrieval(ctx, &wg, retrievalReq)
	}()
	wg.Wait()
	if retrievalErr != nil {
		return "", retrievalErr
	}

	contextText := ""
	if retrievalResults.NewQuery != nil {
		contextText = retrievalResults.NewQuery.Payload["text"]
	}
	if contextText == "" && retrievalResults.CurrentQuery != nil {
		contextText = retrievalResults.CurrentQuery.Payload["text"]
	}
	if contextText == "" && retrievalResults.MultimodelQuery != nil {
		contextText = retrievalResults.MultimodelQuery.Payload["text"]
	}

	finalQuery := query
	if strings.TrimSpace(contextText) != "" {
		finalQuery = query + "\n\nContext:\n" + contextText
	}

	responseAnswer, err := c.llmChat(
		ctx,
		c.PromptAnswer,
		c.Config.OrchestratorService.PreProcessing.Temperature,
		c.Config.OrchestratorService.PreProcessing.Model,
		finalQuery,
		nil,
		imagePath,
		uuid,
	)

	sessionData, err := c.Session.GetSession(uuid)
	sessionData.ChatHistory.AddMessage(
		portsOrchestrator.ChatMessage{
			Role: query,
			Content: responseAnswer.Text,
		},
	)

	if err != nil {
		return "", err
	}

	return responseAnswer.Text, nil
}
