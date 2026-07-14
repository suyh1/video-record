package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/records"
)

type currentRoundResponse struct {
	RoundID        string         `json:"roundId"`
	MediaID        string         `json:"mediaId"`
	SeasonNumber   *int           `json:"seasonNumber"`
	RoundNumber    int            `json:"roundNumber"`
	Status         records.Status `json:"status"`
	Rating         *float64       `json:"rating"`
	Note           *string        `json:"note"`
	ViewingMethod  *string        `json:"viewingMethod"`
	WatchedAt      *time.Time     `json:"watchedAt"`
	Version        int            `json:"version"`
	ProfileVersion int            `json:"profileVersion"`
}

type updateCurrentRoundRequest struct {
	Status           records.Status
	Rating           *int
	RatingSet        bool
	Note             *string
	NoteSet          bool
	ViewingMethod    *string
	ViewingMethodSet bool
	WatchedAt        *time.Time
}

func (handlers recordHandlers) currentRound(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	scope, ok := roundScopeFromRequest(w, r, identity.User.ID)
	if !ok {
		return
	}
	round, err := handlers.service.CurrentRound(r.Context(), scope)
	if err != nil {
		writeRecordError(w, r, err, round.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(round.Version))
	writeJSON(w, http.StatusOK, newCurrentRoundResponse(round))
}

func (handlers recordHandlers) updateCurrentRound(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	expectedVersion, ok := parseIfMatch(w, r)
	if !ok {
		return
	}
	scope, ok := roundScopeFromRequest(w, r, identity.User.ID)
	if !ok {
		return
	}
	request, err := decodeUpdateCurrentRoundRequest(r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	round, err := handlers.service.UpdateRound(r.Context(), records.UpdateRoundInput{
		Scope: scope, Status: request.Status,
		Rating: request.Rating, RatingSet: request.RatingSet,
		Note: request.Note, NoteSet: request.NoteSet,
		ViewingMethod: request.ViewingMethod, ViewingMethodSet: request.ViewingMethodSet,
		CompletedAt: request.WatchedAt, Source: records.SourceManual,
		ExpectedVersion: expectedVersion,
	})
	if err != nil {
		writeRecordError(w, r, err, round.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(round.Version))
	writeJSON(w, http.StatusOK, newCurrentRoundResponse(round))
}

func roundScopeFromRequest(w http.ResponseWriter, r *http.Request, userID string) (records.RoundScope, bool) {
	scope := records.RoundScope{UserID: userID, MediaID: chi.URLParam(r, "mediaID")}
	rawSeason, provided := r.URL.Query()["seasonNumber"]
	if !provided {
		return scope, true
	}
	if len(rawSeason) != 1 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_round_scope")
		return records.RoundScope{}, false
	}
	seasonNumber, err := strconv.Atoi(strings.TrimSpace(rawSeason[0]))
	if err != nil || seasonNumber < 1 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_round_scope")
		return records.RoundScope{}, false
	}
	scope.SeasonNumber = &seasonNumber
	return scope, true
}

func decodeUpdateCurrentRoundRequest(r *http.Request) (updateCurrentRoundRequest, error) {
	var raw struct {
		Status        records.Status  `json:"status"`
		Rating        json.RawMessage `json:"rating"`
		Note          json.RawMessage `json:"note"`
		ViewingMethod json.RawMessage `json:"viewingMethod"`
		WatchedAt     *time.Time      `json:"watchedAt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return updateCurrentRoundRequest{}, err
	}
	request := updateCurrentRoundRequest{Status: raw.Status, WatchedAt: raw.WatchedAt}
	if raw.Rating != nil {
		request.RatingSet = true
		if string(raw.Rating) != "null" {
			var rating float64
			if err := json.Unmarshal(raw.Rating, &rating); err != nil {
				return updateCurrentRoundRequest{}, err
			}
			converted, err := records.RatingFromTen(rating)
			if err != nil {
				return updateCurrentRoundRequest{}, err
			}
			request.Rating = &converted
		}
	}
	if err := decodeNullableString(raw.Note, &request.Note, &request.NoteSet); err != nil {
		return updateCurrentRoundRequest{}, err
	}
	if err := decodeNullableString(raw.ViewingMethod, &request.ViewingMethod, &request.ViewingMethodSet); err != nil {
		return updateCurrentRoundRequest{}, err
	}
	if request.ViewingMethod != nil {
		trimmed := strings.TrimSpace(*request.ViewingMethod)
		request.ViewingMethod = &trimmed
	}
	if request.WatchedAt != nil {
		watchedAt := request.WatchedAt.UTC()
		request.WatchedAt = &watchedAt
	}
	return request, nil
}

func decodeNullableString(raw json.RawMessage, target **string, provided *bool) error {
	if raw == nil {
		return nil
	}
	*provided = true
	if string(raw) == "null" {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*target = &value
	return nil
}

func newCurrentRoundResponse(round records.WatchRound) currentRoundResponse {
	response := currentRoundResponse{
		RoundID: round.ID, MediaID: round.MediaID, SeasonNumber: round.SeasonNumber,
		RoundNumber: round.RoundNumber, Status: round.Status, Note: round.Note,
		ViewingMethod: round.ViewingMethod, WatchedAt: round.CompletedAt,
		Version: round.Version, ProfileVersion: round.ProfileVersion,
	}
	if round.Rating != nil {
		rating := records.RatingToTen(*round.Rating)
		response.Rating = &rating
	}
	return response
}
