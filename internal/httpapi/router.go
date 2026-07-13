package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"video-record/internal/auth"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/media"
	"video-record/internal/records"
	"video-record/internal/storage"
)

type Dependencies struct {
	Logger       *slog.Logger
	Storage      *storage.DB
	Auth         *auth.Service
	CookieSecure bool
	TMDB         *tmdb.Client
	Media        *media.Service
	Records      *records.Service
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
				if dependencies.Records != nil {
					recordAPI := recordHandlers{service: dependencies.Records}
					protected.Get("/collections", recordAPI.collections)
					protected.Get("/records/{mediaID}/events", recordAPI.watchEvents)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Put(
						"/records/{mediaID}", recordAPI.updateState,
					)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Put(
						"/records/{mediaID}/tags", recordAPI.setTags,
					)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post(
						"/collections", recordAPI.createCollection,
					)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post(
						"/collections/{collectionID}/items", recordAPI.addCollectionItem,
					)
					if dependencies.Storage != nil {
						idempotency := newIdempotencyMiddleware(dependencies.Storage)
						protected.With(
							RequireSameOrigin,
							RequireCSRF(dependencies.Auth),
							idempotency.Handle,
						).Post("/records/{mediaID}/events", recordAPI.createWatchEvent)
					}
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Delete(
						"/records/{mediaID}/events/{eventID}", recordAPI.deleteWatchEvent,
					)
				}
				if dependencies.TMDB != nil {
					tmdbAPI := tmdbHandlers{client: dependencies.TMDB}
					protected.Get("/tmdb/status", tmdbAPI.status)
					protected.Get("/tmdb/search", tmdbAPI.search)
					protected.Get("/tmdb/movie/{id}", tmdbAPI.movie)
					protected.Get("/tmdb/tv/{id}", tmdbAPI.tv)
					protected.Get("/tmdb/tv/{id}/season/{season}", tmdbAPI.season)
					protected.Get("/tmdb/tv/{id}/season/{season}/episode/{episode}", tmdbAPI.episode)
				}
				if dependencies.Media != nil && dependencies.TMDB != nil {
					mediaAPI := mediaHandlers{service: dependencies.Media, tmdb: dependencies.TMDB}
					protected.Get("/media/{id}", mediaAPI.get)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post(
						"/media/tmdb/{mediaType}/{externalID}", mediaAPI.createFromTMDB,
					)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post(
						"/media/custom", mediaAPI.createCustom,
					)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post(
						"/media/{id}/tmdb/{mediaType}/{externalID}", mediaAPI.linkTMDB,
					)
				}
			})
		})
	}
	return router
}
