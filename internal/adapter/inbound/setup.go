package inbound

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"rag_imagetotext_texttoimage/internal/util"

	router "rag_imagetotext_texttoimage/internal/adapter/inbound/router"
)

type HTTPHandler struct {
	chat         *router.HTTPHandlerChat
	vectordb     *router.HTTPHandlerVectordb
	trainingFile *router.HTTPHandlerTrainingFile
}

func NewHTTPHandler(
	chatHandler *router.HTTPHandlerChat,
	vectordbHandler *router.HTTPHandlerVectordb,
	trainingFileHandler *router.HTTPHandlerTrainingFile,
) *HTTPHandler {
	return &HTTPHandler{
		chat:         chatHandler,
		vectordb:     vectordbHandler,
		trainingFile: trainingFileHandler,
	}
}

func SetupRouter(handler *HTTPHandler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		util.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/api/v1/orchestrator/chat", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil || handler.chat == nil {
			util.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "chat handler is not configured"})
			return
		}
		handler.chat.HTTPHandlerChatExecute(w, r)
	})
	r.Post("/api/v1/orchestrator/vectordb/collections", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil || handler.vectordb == nil {
			util.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "vectordb handler is not configured"})
			return
		}
		handler.vectordb.HTTPHandlerCreateCollectionExecute(w, r)
	})
	r.Post("/api/v1/orchestrator/vectordb/collections/delete", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil || handler.vectordb == nil {
			util.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "vectordb handler is not configured"})
			return
		}
		handler.vectordb.HTTPHandlerDeleteCollectionExecute(w, r)
	})
	r.Post("/api/v1/orchestrator/vectordb/points/delete-filter", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil || handler.vectordb == nil {
			util.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "vectordb handler is not configured"})
			return
		}
		handler.vectordb.HTTPHandlerDeletePointFilterExecute(w, r)
	})
	r.Post("/api/v1/orchestrator/training-file/process-and-ingest", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil || handler.trainingFile == nil {
			util.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "training file handler is not configured"})
			return
		}
		handler.trainingFile.HTTPHandlerProcessAndIngestExecute(w, r)
	})
	return r
}

func NewHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
}
