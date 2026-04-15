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
	handler := &ChatbotHandler{
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
	if session != nil {
		session.SetOnSessionReleased(func(sessionID string) {
			cleanupSessionTmpDir(sessionID, appLogger)
		})
	}
	return handler
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
	imagePath, err = c.prepareImageForSession(ctx, strings.TrimSpace(imagePath), session_id)
	if err != nil {
		return "", err
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
	retrievalLimit := ragRetrievalTopK(c.Config.OrchestratorService.RAGRetrievalTopK)

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
				Limit:          retrievalLimit,
				WithPayload:    true,
			},
			CurrentQuery: &pb.SearchPointRequest{
				CollectionName: collectionName,
				VectorName:     "text_dense",
				Vector:         embedResults.CurrentText.Embedding,
				Limit:          retrievalLimit,
				WithPayload:    true,
			},
		}
		if embedResults.Image != nil {
			retrievalReq.MultimodalQuery = &pb.SearchPointRequest{
				CollectionName: collectionName,
				VectorName:     "image_dense",
				Vector:         embedResults.Image.Embedding,
				Limit:          retrievalLimit,
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
		contextText, contextSource = selectContextFromRetrieval(strings.TrimSpace(imagePath), retrievalResults)
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
			"retrieval_top_k", retrievalLimit,
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

func ragRetrievalTopK(topK int) uint64 {
	if topK <= 0 {
		return 1
	}
	return uint64(topK)
}

func selectContextFromRetrieval(imagePath string, results RetrievalResult) (string, string) {
	hasImage := strings.TrimSpace(imagePath) != ""
	if hasImage {
		if results.MultimodelQuery != nil && results.MultimodelQuery.Score >= minContextScore {
			return strings.TrimSpace(results.MultimodelQuery.Payload["text"]), "multimodal_query"
		}
		return "", ""
	}
	if results.NewQuery != nil && results.NewQuery.Score >= minContextScore {
		return strings.TrimSpace(results.NewQuery.Payload["text"]), "new_query"
	}
	if results.CurrentQuery != nil && results.CurrentQuery.Score >= minContextScore {
		return strings.TrimSpace(results.CurrentQuery.Payload["text"]), "current_query"
	}
	if results.MultimodelQuery != nil && results.MultimodelQuery.Score >= minContextScore {
		return strings.TrimSpace(results.MultimodelQuery.Payload["text"]), "multimodal_query"
	}
	return "", ""
}
