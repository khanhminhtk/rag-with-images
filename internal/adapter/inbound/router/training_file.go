package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	orchestratordto "rag_imagetotext_texttoimage/internal/application/dtos/orchestrator"
	"rag_imagetotext_texttoimage/internal/application/ports"
	"rag_imagetotext_texttoimage/internal/util"
)

type HTTPHandlerTrainingFile struct {
	useCase ports.TrainingFileUseCase
}

func NewHTTPHandlerTrainingFile(useCase ports.TrainingFileUseCase) *HTTPHandlerTrainingFile {
	return &HTTPHandlerTrainingFile{
		useCase: useCase,
	}
}

func (h *HTTPHandlerTrainingFile) HTTPHandlerProcessAndIngestExecute(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
) {
	_ = ctx
	if h == nil || h.useCase == nil {
		util.WriteJSON(w, http.StatusInternalServerError, orchestratordto.ErrorResponse{Error: "training file handler is not configured"})
		return
	}

	var req orchestratordto.ProcessAndIngestRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "invalid request body"})
		return
	}

	result, err := h.useCase.ProcessAndIngest(r.Context(), &req)
	if err != nil {
		httpStatus := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			httpStatus = http.StatusRequestTimeout
		}
		util.WriteJSON(w, httpStatus, orchestratordto.ErrorResponse{Error: err.Error()})
		return
	}

	util.WriteJSON(w, http.StatusOK, result)
}
