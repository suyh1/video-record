package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"video-record/internal/auth"
	"video-record/internal/storage"
)

type Dependencies struct {
	Logger       *slog.Logger
	Storage      *storage.DB
	Auth         *auth.Service
	CookieSecure bool
}

func NewRouter(dependencies Dependencies) http.Handler {
	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(RequestLogger(dependencies.Logger))
	router.Use(Recoverer(dependencies.Logger))
	router.Get("/healthz", healthz)
	router.Get("/readyz", readyz(dependencies.Storage))
	if dependencies.Auth != nil {
		handlers := authHandlers{service: dependencies.Auth, cookieSecure: dependencies.CookieSecure}
		router.Route("/api/v1", func(api chi.Router) {
			api.Get("/setup/status", handlers.setupStatus)
			api.With(RequireSameOrigin).Post("/setup/admin", handlers.initialize)
			api.With(RequireSameOrigin).Post("/auth/login", handlers.login)
			api.Group(func(protected chi.Router) {
				protected.Use(Authenticate(dependencies.Auth))
				protected.Get("/auth/me", handlers.me)
				protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post("/auth/logout", handlers.logout)
			})
		})
	}
	return router
}
