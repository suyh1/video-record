package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations"
)

const maxIntegrationRequestBytes = 32 << 10

type integrationJobProvisioner interface {
	EnsureJobs(context.Context, string) error
}

type integrationHandlers struct {
	accounts *integrations.AccountRepository
	jobs     integrationJobProvisioner
}

type integrationAccountRequest struct {
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	Token     string `json:"token"`
	UserID    string `json:"userId"`
	AccountID int    `json:"accountId"`
	Timezone  string `json:"timezone"`
}

type integrationAccountResponse struct {
	ID                    string    `json:"id"`
	Provider              string    `json:"provider"`
	Name                  string    `json:"name"`
	CredentialFingerprint string    `json:"credentialFingerprint"`
	Enabled               bool      `json:"enabled"`
	Locked                bool      `json:"locked"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

func (handlers integrationHandlers) list(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	accounts, err := handlers.accounts.List(r.Context(), identity.User.ID)
	if err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	response := make([]integrationAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		response = append(response, newIntegrationAccountResponse(account))
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers integrationHandlers) create(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	request, credentials, err := decodeIntegrationAccountRequest(r)
	if err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	if handlers.jobs == nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	account, err := handlers.accounts.Create(r.Context(), integrations.CreateAccountInput{
		UserID: identity.User.ID, Provider: request.Provider, Name: request.Name,
		BaseURL: request.BaseURL, Credentials: credentials, Enabled: true,
	})
	if err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	if err := handlers.jobs.EnsureJobs(r.Context(), account.ID); err != nil {
		_ = handlers.accounts.Delete(r.Context(), identity.User.ID, account.ID)
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, newIntegrationAccountResponse(account))
}

func (handlers integrationHandlers) disconnect(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	if err := handlers.accounts.Delete(r.Context(), identity.User.ID, chi.URLParam(r, "accountID")); err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeIntegrationAccountRequest(r *http.Request) (integrationAccountRequest, []byte, error) {
	var request integrationAccountRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxIntegrationRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, nil, integrations.ErrInvalidAccount
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return request, nil, integrations.ErrInvalidAccount
	}
	request.Provider = strings.ToLower(strings.TrimSpace(request.Provider))
	request.Name = strings.TrimSpace(request.Name)
	request.BaseURL = strings.TrimSpace(request.BaseURL)
	request.Token = strings.TrimSpace(request.Token)
	request.UserID = strings.TrimSpace(request.UserID)
	request.Timezone = strings.TrimSpace(request.Timezone)
	if request.Name == "" || len(request.Name) > 100 || request.BaseURL == "" ||
		request.Token == "" || len(request.Token) > 4096 || len(request.UserID) > 256 {
		return request, nil, integrations.ErrInvalidAccount
	}
	var credentials any
	switch request.Provider {
	case "jellyfin":
		if request.UserID == "" || request.AccountID != 0 || request.Timezone != "" {
			return request, nil, integrations.ErrInvalidAccount
		}
		credentials = struct {
			Token  string `json:"token"`
			UserID string `json:"userId"`
		}{Token: request.Token, UserID: request.UserID}
	case "emby":
		if request.UserID == "" || request.AccountID != 0 {
			return request, nil, integrations.ErrInvalidAccount
		}
		if request.Timezone != "" {
			if _, err := time.LoadLocation(request.Timezone); err != nil {
				return request, nil, integrations.ErrInvalidAccount
			}
		}
		credentials = struct {
			Token    string `json:"token"`
			UserID   string `json:"userId"`
			Timezone string `json:"timezone,omitempty"`
		}{Token: request.Token, UserID: request.UserID, Timezone: request.Timezone}
	case "plex":
		if request.AccountID <= 0 || request.UserID != "" || request.Timezone != "" {
			return request, nil, integrations.ErrInvalidAccount
		}
		credentials = struct {
			Token     string `json:"token"`
			AccountID int    `json:"accountId"`
		}{Token: request.Token, AccountID: request.AccountID}
	default:
		return request, nil, integrations.ErrInvalidAccount
	}
	contents, err := json.Marshal(credentials)
	if err != nil {
		return request, nil, err
	}
	return request, contents, nil
}

func newIntegrationAccountResponse(account integrations.Account) integrationAccountResponse {
	return integrationAccountResponse{
		ID: account.ID, Provider: account.Provider, Name: account.Name,
		CredentialFingerprint: account.CredentialFingerprint,
		Enabled:               account.Enabled, Locked: account.Locked,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	}
}

func writeIntegrationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, integrations.ErrInvalidAccount):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_integration_account")
	case errors.Is(err, integrations.ErrInvalidCredentialKey), errors.Is(err, integrations.ErrCredentialsLocked):
		writeProblem(w, r, http.StatusLocked, "Locked", "integrations_locked")
	case errors.Is(err, integrations.ErrAccountNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "integration_account_not_found")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}
