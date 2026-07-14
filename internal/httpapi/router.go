package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"video-record/internal/auth"
	"video-record/internal/household"
	"video-record/internal/integrations"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/media"
	"video-record/internal/records"
	statsdomain "video-record/internal/stats"
	"video-record/internal/storage"
	syncdomain "video-record/internal/sync"
)

type Dependencies struct {
	Logger              *slog.Logger
	Storage             *storage.DB
	Auth                *auth.Service
	CookieSecure        bool
	TMDB                *tmdb.Client
	Media               *media.Service
	Records             *records.Service
	Stats               *statsdomain.Service
	Household           *household.Service
	Backup              *storage.BackupManager
	Sync                *syncdomain.CandidateService
	IntegrationAccounts *integrations.AccountRepository
	SyncJobs            *syncdomain.Service
}

func NewRouter(dependencies Dependencies) http.Handler {
	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(RequestLogger(dependencies.Logger))
	router.Use(Recoverer(dependencies.Logger))
	router.Get("/healthz", healthz)
	router.Get("/readyz", readyz(dependencies.Storage))
	if dependencies.Auth != nil {
		handlers := authHandlers{
			service:      dependencies.Auth,
			cookieSecure: dependencies.CookieSecure,
			storage:      dependencies.Storage,
			tmdb:         dependencies.TMDB,
		}
		router.Route("/api/v1", func(api chi.Router) {
			api.NotFound(func(w http.ResponseWriter, r *http.Request) {
				writeProblem(w, r, http.StatusNotFound, "Not Found", "route_not_found")
			})
			api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
				writeProblem(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", "method_not_allowed")
			})
			if dependencies.Storage != nil {
				api.Use(MaintenanceMode(dependencies.Storage))
			}
			api.Get("/setup/status", handlers.setupStatus)
			api.With(RequireSameOrigin).Post("/setup/admin", handlers.initialize)
			api.With(RequireSameOrigin).Post("/auth/login", handlers.login)
			api.Group(func(protected chi.Router) {
				protected.Use(Authenticate(dependencies.Auth))
				protectedWriteMiddleware := []func(http.Handler) http.Handler{
					RequireSameOrigin,
					RequireCSRF(dependencies.Auth),
				}
				if dependencies.Storage != nil {
					idempotency := newIdempotencyMiddleware(dependencies.Storage)
					protectedWriteMiddleware = append(protectedWriteMiddleware, idempotency.Handle)
				}
				protected.Get("/auth/me", handlers.me)
				protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post("/auth/logout", handlers.logout)
				if dependencies.IntegrationAccounts != nil {
					integrationAPI := integrationHandlers{
						accounts: dependencies.IntegrationAccounts,
						jobs:     dependencies.SyncJobs,
					}
					protected.Get("/integrations/accounts", integrationAPI.list)
					protected.With(protectedWriteMiddleware...).Post(
						"/integrations/accounts", integrationAPI.create,
					)
					protected.With(protectedWriteMiddleware...).Delete(
						"/integrations/accounts/{accountID}", integrationAPI.disconnect,
					)
				}
				if dependencies.Backup != nil && dependencies.Storage != nil {
					idempotency := newIdempotencyMiddleware(dependencies.Storage)
					backupAPI := backupHandlers{manager: dependencies.Backup, idempotency: idempotency}
					protected.Get("/backups", backupAPI.list)
					protected.Get("/backups/{filename}", backupAPI.download)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
						Post("/backups", backupAPI.create)
					protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth)).Post("/restore", backupAPI.restore)
				}
				if dependencies.Records != nil {
					recordAPI := recordHandlers{service: dependencies.Records}
					protected.Get("/calendar", recordAPI.calendar)
					protected.Get("/collections", recordAPI.collections)
					protected.Get("/data/export", recordAPI.exportData)
					protected.Get("/library", recordAPI.library)
					protected.Get("/media/search", recordAPI.localSearch)
					protected.Get("/records/{mediaID}", recordAPI.getRecord)
					protected.Get("/records/{mediaID}/events", recordAPI.watchEvents)
					protected.Get("/records/{mediaID}/progress", recordAPI.episodeProgress)
					protected.Get("/records/{mediaID}/tags", recordAPI.tags)
					protected.With(protectedWriteMiddleware...).Put(
						"/records/{mediaID}", recordAPI.updateState,
					)
					protected.With(protectedWriteMiddleware...).Put(
						"/records/{mediaID}/tags", recordAPI.setTags,
					)
					protected.With(protectedWriteMiddleware...).Post(
						"/collections", recordAPI.createCollection,
					)
					protected.With(protectedWriteMiddleware...).Post(
						"/collections/{collectionID}/items", recordAPI.addCollectionItem,
					)
					protected.With(protectedWriteMiddleware...).Put(
						"/collections/{collectionID}/items", recordAPI.replaceCollectionItems,
					)
					if dependencies.Storage != nil {
						idempotency := newIdempotencyMiddleware(dependencies.Storage)
						recordAPI.idempotency = idempotency
						protected.With(
							RequireSameOrigin,
							RequireCSRF(dependencies.Auth),
						).Post("/data/import", recordAPI.importData)
						protected.With(
							RequireSameOrigin,
							RequireCSRF(dependencies.Auth),
							idempotency.Handle,
						).Post("/records/{mediaID}/events", recordAPI.createWatchEvent)
						protected.With(
							RequireSameOrigin,
							RequireCSRF(dependencies.Auth),
							idempotency.Handle,
						).Post("/records/{mediaID}/progress", recordAPI.updateEpisodeProgress)
					}
					protected.With(protectedWriteMiddleware...).Delete(
						"/records/{mediaID}/events/{eventID}", recordAPI.deleteWatchEvent,
					)
				}
				if dependencies.Stats != nil {
					statsAPI := statsHandlers{service: dependencies.Stats}
					protected.Get("/stats", statsAPI.summary)
				}
				if dependencies.Sync != nil {
					syncAPI := syncHandlers{service: dependencies.Sync}
					protected.Get("/sync/status", syncAPI.status)
					protected.Get("/sync/candidates", syncAPI.candidates)
					if dependencies.Storage != nil {
						idempotency := newIdempotencyMiddleware(dependencies.Storage)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/sync/candidates/{candidateID}/confirm", syncAPI.confirm)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/sync/candidates/{candidateID}/rematch", syncAPI.rematch)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/sync/candidates/{candidateID}/ignore", syncAPI.ignore)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/sync/candidates/{candidateID}/custom", syncAPI.custom)
					}
				}
				if dependencies.Household != nil {
					householdAPI := householdHandlers{service: dependencies.Household}
					protected.Get("/household/events", householdAPI.sharedEvents)
					protected.Get("/household/members", householdAPI.members)
					protected.Get("/household/participants", householdAPI.participants)
					protected.Get("/household/records/{ownerID}/{mediaID}", householdAPI.visibleRecord)
					protected.Get("/household/records/{mediaID}/sharing", householdAPI.sharing)
					if dependencies.Storage != nil {
						idempotency := newIdempotencyMiddleware(dependencies.Storage)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/household/members", householdAPI.createMember)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/household/members/{memberID}/reset-password", householdAPI.resetPassword)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Post("/household/members/{memberID}/deactivate", householdAPI.deactivateMember)
						protected.With(RequireSameOrigin, RequireCSRF(dependencies.Auth), idempotency.Handle).
							Put("/household/records/{mediaID}/sharing", householdAPI.updateSharing)
					}
				}
				if dependencies.TMDB != nil {
					tmdbAPI := tmdbHandlers{client: dependencies.TMDB}
					protected.Get("/tmdb/status", tmdbAPI.status)
					protected.Get("/tmdb/connectivity", tmdbAPI.connectivity)
					protected.Get("/tmdb/search", tmdbAPI.search)
					protected.Get("/tmdb/movie/{id}", tmdbAPI.movie)
					protected.Get("/tmdb/tv/{id}", tmdbAPI.tv)
					protected.Get("/tmdb/tv/{id}/season/{season}", tmdbAPI.season)
					protected.Get("/tmdb/tv/{id}/season/{season}/episode/{episode}", tmdbAPI.episode)
					protected.Get("/tmdb/{mediaType}/{id}/credits", tmdbAPI.credits)
				}
				if dependencies.Media != nil && dependencies.TMDB != nil {
					mediaAPI := mediaHandlers{service: dependencies.Media, tmdb: dependencies.TMDB}
					protected.Get("/media/{id}", mediaAPI.get)
					protected.With(protectedWriteMiddleware...).Post(
						"/media/tmdb/{mediaType}/{externalID}", mediaAPI.createFromTMDB,
					)
					protected.With(protectedWriteMiddleware...).Post(
						"/media/custom", mediaAPI.createCustom,
					)
					protected.With(protectedWriteMiddleware...).Post(
						"/media/{id}/tmdb/{mediaType}/{externalID}", mediaAPI.linkTMDB,
					)
				}
			})
		})
	}
	return router
}
