package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Dependencies struct{}

func NewRouter(_ Dependencies) http.Handler {
	router := chi.NewRouter()
	router.Get("/healthz", healthz)
	return router
}
