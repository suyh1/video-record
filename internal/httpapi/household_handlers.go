package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/household"
	"video-record/internal/records"
)

type householdHandlers struct {
	service *household.Service
}

type memberResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

type createMemberRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

type sharingRequest struct {
	ShareRating     bool   `json:"shareRating"`
	ShareReview     bool   `json:"shareReview"`
	SharedReview    string `json:"sharedReview"`
	ExpectedVersion int    `json:"expectedVersion"`
}

type sharingResponse struct {
	MediaID      string  `json:"mediaId"`
	ShareRating  bool    `json:"shareRating"`
	ShareReview  bool    `json:"shareReview"`
	SharedReview *string `json:"sharedReview"`
	Version      int     `json:"version"`
}

type visibleRecordResponse struct {
	OwnerID      string   `json:"ownerId"`
	MediaID      string   `json:"mediaId"`
	Rating       *float64 `json:"rating"`
	PrivateNote  *string  `json:"privateNote"`
	SharedReview *string  `json:"sharedReview"`
}

type sharedEventResponse struct {
	ID           string    `json:"id"`
	MediaID      string    `json:"mediaId"`
	Title        string    `json:"title"`
	WatchedAt    time.Time `json:"watchedAt"`
	Participants []string  `json:"participants"`
}

func (handlers householdHandlers) members(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	members, err := handlers.service.Members(r.Context(), identity.User.ID)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	response := make([]memberResponse, 0, len(members))
	for _, member := range members {
		response = append(response, newMemberResponse(member))
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers householdHandlers) participants(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	participants, err := handlers.service.Participants(r.Context(), identity.User.ID)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	response := make([]memberResponse, 0, len(participants))
	for _, participant := range participants {
		response = append(response, newMemberResponse(participant))
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers householdHandlers) createMember(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request createMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	member, err := handlers.service.CreateMember(r.Context(), identity.User.ID, request.Username, request.Password)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newMemberResponse(member))
}

func (handlers householdHandlers) resetPassword(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	if err := handlers.service.ResetPassword(r.Context(), identity.User.ID, chi.URLParam(r, "memberID"), request.Password); err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handlers householdHandlers) deactivateMember(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	if err := handlers.service.DeactivateMember(r.Context(), identity.User.ID, chi.URLParam(r, "memberID")); err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handlers householdHandlers) updateSharing(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request sharingRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	sharing, err := handlers.service.UpdateSharing(
		r.Context(), identity.User.ID, identity.User.ID, chi.URLParam(r, "mediaID"),
		household.SharingInput{
			ShareRating: request.ShareRating, ShareReview: request.ShareReview,
			SharedReview: request.SharedReview, ExpectedVersion: request.ExpectedVersion,
		},
	)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	w.Header().Set("ETag", quotedVersion(sharing.Version))
	writeJSON(w, http.StatusOK, sharingResponse{
		MediaID: sharing.MediaID, ShareRating: sharing.ShareRating,
		ShareReview: sharing.ShareReview, SharedReview: sharing.SharedReview, Version: sharing.Version,
	})
}

func (handlers householdHandlers) sharing(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	sharing, err := handlers.service.Sharing(
		r.Context(), identity.User.ID, identity.User.ID, chi.URLParam(r, "mediaID"),
	)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	w.Header().Set("ETag", quotedVersion(sharing.Version))
	writeJSON(w, http.StatusOK, sharingResponse{
		MediaID: sharing.MediaID, ShareRating: sharing.ShareRating,
		ShareReview: sharing.ShareReview, SharedReview: sharing.SharedReview, Version: sharing.Version,
	})
}

func (handlers householdHandlers) visibleRecord(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	record, err := handlers.service.VisibleRecord(
		r.Context(), identity.User.ID, chi.URLParam(r, "ownerID"), chi.URLParam(r, "mediaID"),
	)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	response := visibleRecordResponse{
		OwnerID: record.OwnerID, MediaID: record.MediaID,
		PrivateNote: record.PrivateNote, SharedReview: record.SharedReview,
	}
	if record.Rating != nil {
		value := records.RatingToTen(*record.Rating)
		response.Rating = &value
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers householdHandlers) sharedEvents(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	events, err := handlers.service.SharedEvents(r.Context(), identity.User.ID)
	if err != nil {
		writeHouseholdError(w, r, err)
		return
	}
	response := make([]sharedEventResponse, 0, len(events))
	for _, event := range events {
		response = append(response, sharedEventResponse{
			ID: event.ID, MediaID: event.MediaID, Title: event.Title,
			WatchedAt: event.WatchedAt, Participants: event.Participants,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func newMemberResponse(member household.Member) memberResponse {
	return memberResponse{
		ID: member.ID, Username: member.Username, Role: string(member.Role),
		Active: member.Active, CreatedAt: member.CreatedAt,
	}
}

func writeHouseholdError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, household.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "Forbidden", "forbidden")
	case errors.Is(err, household.ErrMemberNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "member_not_found")
	case errors.Is(err, household.ErrRecordNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "record_not_found")
	case errors.Is(err, household.ErrVersionConflict):
		writeProblem(w, r, http.StatusConflict, "Conflict", "version_conflict")
	case errors.Is(err, household.ErrInvalidMember):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_household_input")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}
