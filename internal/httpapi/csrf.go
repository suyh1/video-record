package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"video-record/internal/auth"
)

type identityContextKey struct{}

func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin, err := url.Parse(r.Header.Get("Origin"))
		expectedScheme := "http"
		if r.TLS != nil {
			expectedScheme = "https"
		}
		if err != nil ||
			origin.Scheme != expectedScheme ||
			!strings.EqualFold(origin.Host, r.Host) ||
			origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
			writeProblem(w, r, http.StatusForbidden, "Forbidden", "invalid_origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Authenticate(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
				return
			}
			identity, err := service.Authenticate(r.Context(), cookie.Value)
			if errors.Is(err, auth.ErrInvalidSession) {
				writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
				return
			}
			if err != nil {
				writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
				return
			}
			ctx := context.WithValue(r.Context(), identityContextKey{}, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireCSRF(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := IdentityFromContext(r.Context())
			if !ok || !service.ValidateCSRF(identity, r.Header.Get("X-CSRF-Token")) {
				writeProblem(w, r, http.StatusForbidden, "Forbidden", "invalid_csrf")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func IdentityFromContext(ctx context.Context) (auth.Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(auth.Identity)
	return identity, ok
}
