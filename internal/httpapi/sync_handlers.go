package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations"
	syncdomain "video-record/internal/sync"
)

type syncHandlers struct {
	service *syncdomain.CandidateService
}

type syncCandidateResponse struct {
	ID              string                     `json:"id"`
	AccountID       string                     `json:"accountId"`
	ExternalEventID string                     `json:"externalEventId"`
	Status          syncdomain.CandidateStatus `json:"status"`
	MediaID         string                     `json:"mediaId,omitempty"`
	EpisodeID       string                     `json:"episodeId,omitempty"`
	Event           syncCandidateEventResponse `json:"event"`
	Evidence        []syncdomain.MatchEvidence `json:"evidence"`
	Options         []syncdomain.MatchOption   `json:"options"`
	CreatedAt       time.Time                  `json:"createdAt"`
	UpdatedAt       time.Time                  `json:"updatedAt"`
}

type syncCandidateEventResponse struct {
	PlayedAt       time.Time              `json:"playedAt"`
	Duration       int                    `json:"durationSeconds"`
	Position       int                    `json:"positionSeconds"`
	ProviderItemID string                 `json:"providerItemId"`
	MediaType      integrations.MediaType `json:"mediaType"`
	Title          string                 `json:"title"`
	OriginalTitle  string                 `json:"originalTitle,omitempty"`
	Year           int                    `json:"year,omitempty"`
	SeasonNumber   int                    `json:"seasonNumber,omitempty"`
	EpisodeNumber  int                    `json:"episodeNumber,omitempty"`
}

type rematchCandidateRequest struct {
	MediaID   string `json:"mediaId"`
	EpisodeID string `json:"episodeId"`
}

func (handlers syncHandlers) status(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	status, err := handlers.service.Status(r.Context(), identity.User.ID)
	if err != nil {
		writeSyncError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handlers syncHandlers) candidates(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	status := syncdomain.CandidateStatus(r.URL.Query().Get("status"))
	candidates, err := handlers.service.Candidates(r.Context(), identity.User.ID, status)
	if err != nil {
		writeSyncError(w, r, err)
		return
	}
	response := make([]syncCandidateResponse, 0, len(candidates))
	for _, candidate := range candidates {
		response = append(response, newSyncCandidateResponse(candidate))
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers syncHandlers) confirm(w http.ResponseWriter, r *http.Request) {
	handlers.writeCandidateAction(w, r, func(userID, candidateID string) (syncdomain.Candidate, error) {
		return handlers.service.Confirm(r.Context(), userID, candidateID)
	})
}

func (handlers syncHandlers) rematch(w http.ResponseWriter, r *http.Request) {
	var request rematchCandidateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	handlers.writeCandidateAction(w, r, func(userID, candidateID string) (syncdomain.Candidate, error) {
		return handlers.service.Rematch(r.Context(), userID, candidateID, request.MediaID, request.EpisodeID)
	})
}

func (handlers syncHandlers) ignore(w http.ResponseWriter, r *http.Request) {
	handlers.writeCandidateAction(w, r, func(userID, candidateID string) (syncdomain.Candidate, error) {
		return handlers.service.Ignore(r.Context(), userID, candidateID)
	})
}

func (handlers syncHandlers) custom(w http.ResponseWriter, r *http.Request) {
	var request syncdomain.CustomMediaInput
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	handlers.writeCandidateAction(w, r, func(userID, candidateID string) (syncdomain.Candidate, error) {
		return handlers.service.CreateCustom(r.Context(), userID, candidateID, request)
	})
}

func (handlers syncHandlers) writeCandidateAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(string, string) (syncdomain.Candidate, error),
) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	candidate, err := action(identity.User.ID, chi.URLParam(r, "candidateID"))
	if err != nil {
		writeSyncError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newSyncCandidateResponse(candidate))
}

func newSyncCandidateResponse(candidate syncdomain.Candidate) syncCandidateResponse {
	return syncCandidateResponse{
		ID: candidate.ID, AccountID: candidate.AccountID,
		ExternalEventID: candidate.ExternalEventID, Status: candidate.Status,
		MediaID: candidate.MediaID, EpisodeID: candidate.EpisodeID,
		Event: syncCandidateEventResponse{
			PlayedAt: candidate.Event.PlayedAt,
			Duration: candidate.Event.DurationSeconds, Position: candidate.Event.PositionSeconds,
			ProviderItemID: candidate.Event.Item.ProviderItemID,
			MediaType:      candidate.Event.Item.MediaType, Title: candidate.Event.Item.Title,
			OriginalTitle: candidate.Event.Item.OriginalTitle, Year: candidate.Event.Item.Year,
			SeasonNumber: candidate.Event.Item.SeasonNumber, EpisodeNumber: candidate.Event.Item.EpisodeNumber,
		},
		Evidence: candidate.Evidence, Options: candidate.Options,
		CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt,
	}
}

func writeSyncError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, syncdomain.ErrCandidateNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "sync_candidate_not_found")
	case errors.Is(err, syncdomain.ErrSyncForbidden):
		writeProblem(w, r, http.StatusForbidden, "Forbidden", "forbidden")
	case errors.Is(err, syncdomain.ErrInvalidCandidate):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_sync_candidate")
	case errors.Is(err, syncdomain.ErrCandidateResolved):
		writeProblem(w, r, http.StatusConflict, "Conflict", "sync_candidate_resolved")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}
