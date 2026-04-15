package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	orchestratordto "rag_imagetotext_texttoimage/internal/application/dtos/orchestrator"
	orchestratoruc "rag_imagetotext_texttoimage/internal/application/use_cases/orchestrator"
	"rag_imagetotext_texttoimage/internal/util"

	pb "rag_imagetotext_texttoimage/proto"
)

type HTTPHandlerVectordb struct {
	vectordb      *orchestratoruc.VectordbHandler
	vectordbSetup util.VectordbSetup
}

func NewHTTPHandlerVectordb(vectordb *orchestratoruc.VectordbHandler, vectordbSetup util.VectordbSetup) *HTTPHandlerVectordb {
	return &HTTPHandlerVectordb{
		vectordb:      vectordb,
		vectordbSetup: vectordbSetup,
	}
}

func (h *HTTPHandlerVectordb) HTTPHandlerCreateCollectionExecute(
	w http.ResponseWriter,
	r *http.Request,
) {
	if h == nil || h.vectordb == nil {
		util.WriteJSON(w, http.StatusInternalServerError, orchestratordto.ErrorResponse{Error: "vectordb handler is not configured"})
		return
	}

	var req orchestratordto.CreateCollectionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "collection name is required"})
		return
	}

	status, err := h.vectordb.CreateCollection(r.Context(), &pb.SchemaCollection{
		Name: req.Name,
		Vectors: []*pb.CollectionVectorConfig{
			{
				Name:     "text_dense",
				Size:     h.vectordbSetup.TextVectorSize,
				Distance: strings.TrimSpace(h.vectordbSetup.TextVectorDistance),
			},
			{
				Name:     "image_dense",
				Size:     h.vectordbSetup.ImageVectorSize,
				Distance: strings.TrimSpace(h.vectordbSetup.ImageVectorDistance),
			},
		},
		Shards:            h.vectordbSetup.Shards,
		ReplicationFactor: h.vectordbSetup.ReplicationFactor,
		OnDiskPayload:     h.vectordbSetup.OnDiskPayload,
		OptimizersMemmap:  h.vectordbSetup.OptimizersMemmap,
	})
	if err != nil {
		httpStatus := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			httpStatus = http.StatusRequestTimeout
		}
		util.WriteJSON(w, httpStatus, orchestratordto.ErrorResponse{Error: err.Error()})
		return
	}

	util.WriteJSON(w, http.StatusOK, orchestratordto.CreateCollectionResponse{Name: req.Name, Status: status})
}

func (h *HTTPHandlerVectordb) HTTPHandlerDeleteCollectionExecute(
	w http.ResponseWriter,
	r *http.Request,
) {
	if h == nil || h.vectordb == nil {
		util.WriteJSON(w, http.StatusInternalServerError, orchestratordto.ErrorResponse{Error: "vectordb handler is not configured"})
		return
	}

	var req orchestratordto.DeleteCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "collection name is required"})
		return
	}

	status, err := h.vectordb.DeleteCollection(r.Context(), req.Name)
	if err != nil {
		httpStatus := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			httpStatus = http.StatusRequestTimeout
		}
		util.WriteJSON(w, httpStatus, orchestratordto.ErrorResponse{Error: err.Error()})
		return
	}

	util.WriteJSON(w, http.StatusOK, orchestratordto.DeleteCollectionResponse{Name: req.Name, Status: status})
}

func (h *HTTPHandlerVectordb) HTTPHandlerDeletePointFilterExecute(
	w http.ResponseWriter,
	r *http.Request,
) {
	if h == nil || h.vectordb == nil {
		util.WriteJSON(w, http.StatusInternalServerError, orchestratordto.ErrorResponse{Error: "vectordb handler is not configured"})
		return
	}

	var req orchestratordto.DeletePointFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "invalid request body"})
		return
	}

	req.CollectionName = strings.TrimSpace(req.CollectionName)
	if req.CollectionName == "" {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "collection name is required"})
		return
	}
	if len(req.Filter.Must) == 0 && len(req.Filter.Should) == 0 && len(req.Filter.MustNot) == 0 {
		util.WriteJSON(w, http.StatusBadRequest, orchestratordto.ErrorResponse{Error: "filter is required"})
		return
	}

	status, err := h.vectordb.DeletePointFilter(r.Context(), &pb.DeletePointFilterRequest{
		CollectionName: req.CollectionName,
		Filter:         toPBFilter(req.Filter),
	})
	if err != nil {
		httpStatus := http.StatusInternalServerError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			httpStatus = http.StatusRequestTimeout
		}
		util.WriteJSON(w, httpStatus, orchestratordto.ErrorResponse{Error: err.Error()})
		return
	}

	util.WriteJSON(w, http.StatusOK, orchestratordto.DeletePointFilterResponse{
		CollectionName: req.CollectionName,
		Status:         status,
	})
}

func toPBFilter(in orchestratordto.Filter) *pb.Filter {
	return &pb.Filter{
		Must:    toPBFieldConditions(in.Must),
		Should:  toPBFieldConditions(in.Should),
		MustNot: toPBFieldConditions(in.MustNot),
	}
}

func toPBFieldConditions(in []orchestratordto.FieldCondition) []*pb.FieldCondition {
	out := make([]*pb.FieldCondition, 0, len(in))
	for _, item := range in {
		cond := &pb.FieldCondition{
			Key:          strings.TrimSpace(item.Key),
			Operator:     strings.TrimSpace(item.Operator),
			StringValues: item.StringValues,
			IntValues:    item.IntValues,
		}
		switch {
		case item.StringValue != nil:
			cond.ScalarValue = &pb.FieldCondition_StringValue{StringValue: *item.StringValue}
		case item.BoolValue != nil:
			cond.ScalarValue = &pb.FieldCondition_BoolValue{BoolValue: *item.BoolValue}
		case item.IntValue != nil:
			cond.ScalarValue = &pb.FieldCondition_IntValue{IntValue: *item.IntValue}
		}
		out = append(out, cond)
	}
	return out
}
