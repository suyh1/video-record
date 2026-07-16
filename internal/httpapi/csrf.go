package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"video-record/internal/auth"
)

type identityContextKey struct{}

func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin, err := url.Parse(r.Header.Get("Origin"))
		expectedScheme, schemeOK := effectiveRequestScheme(r)
		if err != nil ||
			!schemeOK ||
			!strings.EqualFold(origin.Scheme, expectedScheme) ||
			!strings.EqualFold(origin.Host, r.Host) ||
			origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
			writeProblem(w, r, http.StatusForbidden, "Forbidden", "invalid_origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func effectiveRequestScheme(r *http.Request) (string, bool) {
	if r.TLS != nil {
		return "https", true
	}
	forwarded, forwardedSet, forwardedOK := forwardedProto(r.Header.Values("Forwarded"))
	xForwarded, xForwardedSet, xForwardedOK := firstProxyProto(r.Header.Values("X-Forwarded-Proto"))
	if !forwardedOK || !xForwardedOK || (forwardedSet && xForwardedSet && forwarded != xForwarded) {
		return "", false
	}
	if forwardedSet {
		return forwarded, true
	}
	if xForwardedSet {
		return xForwarded, true
	}
	return "http", true
}

func forwardedProto(values []string) (string, bool, bool) {
	if len(values) == 0 {
		return "", false, true
	}
	firstElement := strings.SplitN(strings.Join(values, ","), ",", 2)[0]
	proto := ""
	for _, parameter := range strings.Split(firstElement, ";") {
		key, value, found := strings.Cut(parameter, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "proto") {
			continue
		}
		if proto != "" {
			return "", true, false
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `"`) {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return "", true, false
			}
			value = unquoted
		} else if strings.Contains(value, `"`) {
			return "", true, false
		}
		var valid bool
		proto, valid = validProxyProto(value)
		if !valid {
			return "", true, false
		}
	}
	return proto, proto != "", true
}

func firstProxyProto(values []string) (string, bool, bool) {
	if len(values) == 0 {
		return "", false, true
	}
	value := strings.TrimSpace(strings.SplitN(strings.Join(values, ","), ",", 2)[0])
	proto, valid := validProxyProto(value)
	return proto, true, valid
}

func validProxyProto(value string) (string, bool) {
	proto := strings.ToLower(strings.TrimSpace(value))
	return proto, proto == "http" || proto == "https"
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
