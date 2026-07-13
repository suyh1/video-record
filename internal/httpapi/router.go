package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Dependencies struct {
	Logger *slog.Logger
}

func NewRouter(dependencies Dependencies) http.Handler {
	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(RequestLogger(dependencies.Logger))
	router.Use(Recoverer(dependencies.Logger))
	router.Get("/healthz", healthz)
	return router
}
