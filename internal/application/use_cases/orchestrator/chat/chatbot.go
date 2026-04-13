package chat

import (
	"context"
	"errors"
	"strings"
	"sync"

	portsOrchestrator "rag_imagetotext_texttoimage/internal/application/ports/orchestrator"
	"rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
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
	appLogger util.Logger,
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
		appLogger:            appLogger,
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

const minContextScore = 0.55

func (c *ChatbotHandler) Execute(
	ctx context.Context,
	query string,
	imagePath string,
	session_id string,
	uuid string,
) (string, error) {
	exist, err := c.Session.SessionExists(session_id)
	if err != nil {
		if c.appLogger != nil {
			c.appLogger.Error("internal.application.use_cases.orchestrator.chat.SessionExists failed", err)
		}
		return "", err
	}
	if !exist {
		if err := c.Session.CreateSession(session_id); err != nil {
			if c.appLogger != nil {
				c.appLogger.Error("internal.application.use_cases.orchestrator.chat.CreateSession failed", err)
			}
			return "", err
		}
	}

	topK := memoryTopK(c.Config.OrchestratorService.MemoryHistoryTopK)
	sessionHistory, err := c.getSessionHistory(session_id)
	if err != nil {
		return "", err
	}
	if c.appLogger != nil {
		c.appLogger.Info(
			"internal.application.use_cases.orchestrator.chat.Execute incoming request",
			"session_id", session_id,
			"query", query,
			"normalized_query", normalizeIntentText(query),
			"history_count", len(sessionHistory),
		)
	}

	responsePreprocess, err := c.llmChat(
		ctx,
		c.PromptPreprocessing,
		c.Config.OrchestratorService.PreProcessing.Temperature,
		c.Config.OrchestratorService.PreProcessing.Model,
		query,
		c.Config.OrchestratorService.PreProcessing.StructOutput,
		imagePath,
		session_id,
		"preprocess",
	)
	if err != nil {
		return "", err
	}

	var executeQueries ExecuteQueries
	executeQueries.CurrentQuery.TextDense = strings.TrimSpace(responsePreprocess.Json["CurrentQuery"])
	executeQueries.NewQuery.TextDense = strings.TrimSpace(responsePreprocess.Json["NewQuery"])
	skipRetrieval := shouldSkipRetrieval(query, imagePath)
	if c.appLogger != nil {
		c.appLogger.Info(
			"internal.application.use_cases.orchestrator.chat.Execute preprocess output",
			"session_id", session_id,
			"current_query", executeQueries.CurrentQuery.TextDense,
			"new_query", executeQueries.NewQuery.TextDense,
			"skip_retrieval", skipRetrieval,
		)
	}
	if strings.TrimSpace(imagePath) != "" {
		executeQueries.MultimodalQuery = &QueryPayload{ImageDense: imagePath}
	}

	contextText := ""
	contextSource := ""
	newQueryScore := float32(-1)
	currentQueryScore := float32(-1)
	multimodalQueryScore := float32(-1)

	if !skipRetrieval {
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

		collectionName := strings.TrimSpace(uuid)
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

		newQueryScore = retrievalScore(retrievalResults.NewQuery)
		currentQueryScore = retrievalScore(retrievalResults.CurrentQuery)
		multimodalQueryScore = retrievalScore(retrievalResults.MultimodelQuery)

		if retrievalResults.NewQuery != nil && retrievalResults.NewQuery.Score >= minContextScore {
			contextText = retrievalResults.NewQuery.Payload["text"]
			contextSource = "new_query"
		}
		if contextText == "" && retrievalResults.CurrentQuery != nil && retrievalResults.CurrentQuery.Score >= minContextScore {
			contextText = retrievalResults.CurrentQuery.Payload["text"]
			contextSource = "current_query"
		}
		if contextText == "" && retrievalResults.MultimodelQuery != nil && retrievalResults.MultimodelQuery.Score >= minContextScore {
			contextText = retrievalResults.MultimodelQuery.Payload["text"]
			contextSource = "multimodal_query"
		}
	}

	finalQuery := query
	if strings.TrimSpace(contextText) != "" {
		finalQuery = query + "\n\nContext:\n" + contextText
	}
	if c.appLogger != nil {
		c.appLogger.Info(
			"internal.application.use_cases.orchestrator.chat.Execute full prompt for answer",
			"session_id", session_id,
			"collection_name", strings.TrimSpace(uuid),
			"new_query_score", newQueryScore,
			"current_query_score", currentQueryScore,
			"multimodal_query_score", multimodalQueryScore,
			"context_source", contextSource,
			"skip_retrieval", skipRetrieval,
			"min_context_score", minContextScore,
			"full_prompt", finalQuery,
		)
	}

	responseAnswer, err := c.llmChat(
		ctx,
		c.PromptAnswer,
		c.Config.OrchestratorService.PreProcessing.Temperature,
		c.Config.OrchestratorService.PreProcessing.Model,
		finalQuery,
		nil,
		imagePath,
		session_id,
		"answer",
	)
	if err != nil {
		return "", err
	}
	if c.appLogger != nil {
		c.appLogger.Info(
			"internal.application.use_cases.orchestrator.chat.Execute answer raw",
			"session_id", session_id,
			"answer_raw", responseAnswer.Text,
		)
	}

	finalAnswer := responseAnswer.Text
	postprocessRaw := ""
	postprocessReason := "postprocess_prompt_empty"
	if strings.TrimSpace(c.PromptPostprocessing) != "" {
		postprocessReason = "postprocess_skipped"
		postprocessInput := "Cau hoi nguoi dung: " + query + "\n\nCau tra loi goc cua chatbot:\n" + responseAnswer.Text
		responsePostprocess, postErr := c.llmChat(
			ctx,
			c.PromptPostprocessing,
			c.Config.OrchestratorService.PreProcessing.Temperature,
			c.Config.OrchestratorService.PreProcessing.Model,
			postprocessInput,
			nil,
			"",
			session_id,
			"postprocess",
		)
		if postErr != nil {
			postprocessReason = "postprocess_error"
			if c.appLogger != nil {
				c.appLogger.Error("internal.application.use_cases.orchestrator.chat.Execute postprocessing failed", postErr, "session_id", session_id)
			}
		} else if responsePostprocess != nil && strings.TrimSpace(responsePostprocess.Text) != "" {
			postRaw := strings.TrimSpace(responsePostprocess.Text)
			postprocessRaw = postRaw
			if c.appLogger != nil {
				c.appLogger.Info(
					"internal.application.use_cases.orchestrator.chat.Execute postprocess raw",
					"session_id", session_id,
					"postprocess_raw", postRaw,
				)
			}
			lastAssistant := latestAssistantMessage(sessionHistory, topK)
			if ok, reason := shouldAcceptPostprocess(responseAnswer.Text, postRaw, lastAssistant); ok {
				finalAnswer = postRaw
				postprocessReason = "accepted"
			} else if c.appLogger != nil {
				postprocessReason = reason
				c.appLogger.Info(
					"internal.application.use_cases.orchestrator.chat.Execute postprocess fallback",
					"session_id", session_id,
					"reason", reason,
				)
			} else {
				postprocessReason = reason
			}
		}
	}
	c.logAnswerPipeline("knowledge", responseAnswer.Text, postprocessRaw, postprocessReason, finalAnswer)
	if err := c.appendConversation(session_id, query, finalAnswer); err != nil {
		return "", err
	}

	return finalAnswer, nil
}

func (c *ChatbotHandler) appendConversation(sessionID string, userQuery string, assistantAnswer string) error {
	sessionData, err := c.Session.GetSession(sessionID)
	if err != nil {
		return err
	}
	if sessionData.ChatHistory == nil {
		return nil
	}
	if err := sessionData.ChatHistory.AddMessage(portsOrchestrator.ChatMessage{
		Role:    "user",
		Content: userQuery,
	}); err != nil {
		return err
	}
	return sessionData.ChatHistory.AddMessage(portsOrchestrator.ChatMessage{
		Role:    "assistant",
		Content: assistantAnswer,
	})
}

func (c *ChatbotHandler) getSessionHistory(sessionID string) ([]portsOrchestrator.ChatMessage, error) {
	sessionData, err := c.Session.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sessionData.ChatHistory == nil {
		return []portsOrchestrator.ChatMessage{}, nil
	}
	history, err := sessionData.ChatHistory.GetChatHistory()
	if err != nil {
		return nil, err
	}
	return history, nil
}

func (c *ChatbotHandler) logAnswerPipeline(answerSource, answerRaw, postRaw, postReason, finalAnswer string) {
	if c.appLogger == nil {
		return
	}
	c.appLogger.Info(
		"internal.application.use_cases.orchestrator.chat.Execute answer pipeline",
		"answer_source", answerSource,
		"answer_raw", answerRaw,
		"postprocess_raw", postRaw,
		"postprocess_reason", postReason,
		"final_answer", finalAnswer,
	)
}

func retrievalScore(item *pb.SearchResultItem) float32 {
	if item == nil {
		return -1
	}
	return item.Score
}
