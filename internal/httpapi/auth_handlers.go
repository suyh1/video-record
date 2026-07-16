package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"video-record/internal/auth"
	"video-record/internal/integrations/tmdb"
	"video-record/internal/storage"
)

const SessionCookieName = "video_record_session"

type authHandlers struct {
	service      *auth.Service
	cookieSecure bool
	storage      *storage.DB
	tmdb         *tmdb.Client
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID       string    `json:"id"`
	Username string    `json:"username"`
	Role     auth.Role `json:"role"`
}

func (handlers authHandlers) setupStatus(w http.ResponseWriter, r *http.Request) {
	initialized, err := handlers.service.IsInitialized(r.Context())
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	storageReady := false
	if handlers.storage != nil {
		storageReady = handlers.storage.Ready(r.Context()) == nil
	}
	tmdbConfigured := handlers.tmdb != nil && handlers.tmdb.Configured()
	writeJSON(w, http.StatusOK, map[string]bool{
		"initialized":    initialized,
		"storageReady":   storageReady,
		"tmdbConfigured": tmdbConfigured,
	})
}

func (handlers authHandlers) initialize(w http.ResponseWriter, r *http.Request) {
	var request credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	user, err := handlers.service.Initialize(r.Context(), request.Username, request.Password)
	switch {
	case errors.Is(err, auth.ErrInitializationClosed):
		writeProblem(w, r, http.StatusConflict, "Conflict", "initialization_closed")
		return
	case errors.Is(err, auth.ErrInvalidInput):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_input")
		return
	case err != nil:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, newUserResponse(user))
}

func (handlers authHandlers) login(w http.ResponseWriter, r *http.Request) {
	var request credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	session, err := handlers.service.Login(r.Context(), request.Username, request.Password, clientBucket(r))
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		writeProblem(w, r, http.StatusTooManyRequests, "Too Many Requests", "login_rate_limited")
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "invalid_credentials")
		return
	case err != nil:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	identity, err := handlers.service.Authenticate(r.Context(), session.Token)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	http.SetCookie(w, handlers.sessionCookie(session.Token, session.ExpiresAt))
	writeJSON(w, http.StatusOK, map[string]any{
		"user":      newUserResponse(identity.User),
		"csrfToken": session.CSRFToken,
	})
}

func (handlers authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil && !errors.Is(err, http.ErrNoCookie) {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	if err == nil && cookie.Value != "" {
		if err := handlers.service.Revoke(r.Context(), cookie.Value); err != nil {
			writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
			return
		}
	}
	expired := handlers.sessionCookie("", time.Unix(1, 0).UTC())
	expired.MaxAge = -1
	http.SetCookie(w, expired)
	w.WriteHeader(http.StatusNoContent)
}

func (handlers authHandlers) me(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	writeJSON(w, http.StatusOK, newUserResponse(identity.User))
}

func (handlers authHandlers) sessionCookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   handlers.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func newUserResponse(user auth.User) userResponse {
	return userResponse{ID: user.ID, Username: user.Username, Role: user.Role}
}

func clientBucket(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
