package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"video-record/internal/storage"
)

type Dependencies struct {
	Logger  *slog.Logger
	Storage *storage.DB
}

func NewRouter(dependencies Dependencies) http.Handler {
	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(RequestLogger(dependencies.Logger))
	router.Use(Recoverer(dependencies.Logger))
	router.Get("/healthz", healthz)
	router.Get("/readyz", readyz(dependencies.Storage))
	return router
}
