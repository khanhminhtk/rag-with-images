package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"rag_imagetotext_texttoimage/internal/application/dtos/orchestrator"
	"rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator/chat"
	"rag_imagetotext_texttoimage/internal/util"
)

type HTTPHandlerChat struct {
	chatbot *chat.ChatbotHandler
}

func NewHTTPHandlerChat(chatbot *chat.ChatbotHandler) *HTTPHandlerChat {
	return &HTTPHandlerChat{
		chatbot: chatbot,
	}
}

func (H *HTTPHandlerChat) HTTPHandlerChatExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
) {
	_ = ctx
	if H == nil || H.chatbot == nil {
		util.WriteJSON(w, http.StatusInternalServerError, orchestrator.ErrorResponse{Error: "chatbot handler is not configured"})
		return
	}

	var req orchestrator.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, orchestrator.ErrorResponse{Error: "invalid request body"})
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		util.WriteJSON(w, http.StatusBadRequest, orchestrator.ErrorResponse{Error: "query is required"})
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.SessionID == "" {
		util.WriteJSON(w, http.StatusBadRequest, orchestrator.ErrorResponse{Error: "SessionID is required"})
		return
	}

	req.Uuid = strings.TrimSpace(req.Uuid)
	if req.Uuid == "" {
		util.WriteJSON(w, http.StatusBadRequest, orchestrator.ErrorResponse{Error: "Uuid is required"})
		return
	}

	answer, err := H.chatbot.Execute(r.Context(), req.Query, strings.TrimSpace(req.ImagePath), req.SessionID, req.Uuid)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusRequestTimeout
		}
		util.WriteJSON(w, status, orchestrator.ErrorResponse{Error: err.Error()})
		return
	}

	fmt.Printf("TEST: %v\n", answer)

	util.WriteJSON(w, http.StatusOK, orchestrator.ChatResponse{
		Answer:    answer,
		SessionID: req.SessionID,
	})

}
