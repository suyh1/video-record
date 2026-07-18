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
	StartedAt      *time.Time     `json:"startedAt"`
	Version        int            `json:"version"`
	ProfileVersion int            `json:"profileVersion"`
	ParticipantIDs []string       `json:"participantIds"`
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
	StartedAt        *time.Time
	StartedAtSet     bool
	ParticipantIDs   []string
}

type archivedRoundResponse struct {
	RoundID       string         `json:"roundId"`
	MediaID       string         `json:"mediaId"`
	SeasonNumber  *int           `json:"seasonNumber"`
	RoundNumber   int            `json:"roundNumber"`
	Status        records.Status `json:"status"`
	Rating        *float64       `json:"rating"`
	Note          *string        `json:"note"`
	ViewingMethod *string        `json:"viewingMethod"`
	WatchedAt     *time.Time     `json:"watchedAt"`
	ArchivedAt    *time.Time     `json:"archivedAt"`
}

type roundSummaryResponse struct {
	RoundID      string     `json:"roundId"`
	MediaID      string     `json:"mediaId"`
	SeasonNumber *int       `json:"seasonNumber"`
	RoundNumber  int        `json:"roundNumber"`
	WatchedAt    *time.Time `json:"watchedAt"`
	Rating       *float64   `json:"rating"`
}

type roundHistoryResponse struct {
	Rounds []roundSummaryResponse `json:"rounds"`
}

type archivedRoundDetailResponse struct {
	Round    archivedRoundResponse `json:"round"`
	Episodes []episodeProgressItem `json:"episodes"`
}

type rewatchRoundResponse struct {
	Archived archivedRoundResponse `json:"archived"`
	Current  currentRoundResponse  `json:"current"`
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
		CompletedAt: request.WatchedAt,
		StartedAt: request.StartedAt, StartedAtSet: request.StartedAtSet,
		Source: records.SourceManual,
		ExpectedVersion: expectedVersion,
		ParticipantIDs:  request.ParticipantIDs,
	})
	if err != nil {
		writeRecordError(w, r, err, round.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(round.Version))
	writeJSON(w, http.StatusOK, newCurrentRoundResponse(round))
}

func (handlers recordHandlers) roundHistory(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	scope, ok := roundScopeFromRequest(w, r, identity.User.ID)
	if !ok {
		return
	}
	history, err := handlers.service.RoundHistory(r.Context(), scope)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	response := roundHistoryResponse{Rounds: make([]roundSummaryResponse, 0, len(history))}
	for _, round := range history {
		item := roundSummaryResponse{
			RoundID: round.ID, MediaID: round.MediaID, SeasonNumber: round.SeasonNumber,
			RoundNumber: round.RoundNumber, WatchedAt: round.CompletedAt,
		}
		if round.Rating != nil {
			rating := records.RatingToTen(*round.Rating)
			item.Rating = &rating
		}
		response.Rounds = append(response.Rounds, item)
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers recordHandlers) archivedRoundDetail(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	scope, ok := roundScopeFromRequest(w, r, identity.User.ID)
	if !ok {
		return
	}
	detail, err := handlers.service.RoundDetail(r.Context(), scope, chi.URLParam(r, "roundID"))
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	response := archivedRoundDetailResponse{
		Round:    newArchivedRoundResponse(detail.Round),
		Episodes: make([]episodeProgressItem, 0, len(detail.Episodes)),
	}
	for _, episode := range detail.Episodes {
		response.Episodes = append(response.Episodes, newEpisodeProgressItem(episode))
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers recordHandlers) clearCurrentRoundFields(w http.ResponseWriter, r *http.Request) {
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
	round, err := handlers.service.ClearCurrentRoundFields(r.Context(), scope, expectedVersion)
	if err != nil {
		writeRecordError(w, r, err, round.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(round.Version))
	writeJSON(w, http.StatusOK, newCurrentRoundResponse(round))
}

func (handlers recordHandlers) removeFromLibrary(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	mediaID := chi.URLParam(r, "mediaID")
	if err := handlers.service.RemoveFromLibrary(r.Context(), identity.User.ID, mediaID); err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handlers recordHandlers) deleteArchivedRound(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	scope, ok := roundScopeFromRequest(w, r, identity.User.ID)
	if !ok {
		return
	}
	if err := handlers.service.DeleteArchivedRound(r.Context(), scope, chi.URLParam(r, "roundID")); err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handlers recordHandlers) startRewatch(w http.ResponseWriter, r *http.Request) {
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
	result, err := handlers.service.StartRewatch(r.Context(), records.RewatchInput{
		Scope: scope, ExpectedVersion: expectedVersion,
	})
	if err != nil {
		writeRecordError(w, r, err, expectedVersion)
		return
	}
	w.Header().Set("ETag", quotedVersion(result.Current.Version))
	writeJSON(w, http.StatusOK, rewatchRoundResponse{
		Archived: newArchivedRoundResponse(result.Archived),
		Current:  newCurrentRoundResponse(result.Current),
	})
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
		Status         records.Status  `json:"status"`
		Rating         json.RawMessage `json:"rating"`
		Note           json.RawMessage `json:"note"`
		ViewingMethod  json.RawMessage `json:"viewingMethod"`
		WatchedAt      *time.Time      `json:"watchedAt"`
		StartedAt      json.RawMessage `json:"startedAt"`
		ParticipantIDs []string        `json:"participantIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return updateCurrentRoundRequest{}, err
	}
	request := updateCurrentRoundRequest{
		Status: raw.Status, WatchedAt: raw.WatchedAt, ParticipantIDs: raw.ParticipantIDs,
	}
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
	if raw.StartedAt != nil {
		request.StartedAtSet = true
		if string(raw.StartedAt) != "null" {
			var startedAt time.Time
			if err := json.Unmarshal(raw.StartedAt, &startedAt); err != nil {
				return updateCurrentRoundRequest{}, err
			}
			startedAt = startedAt.UTC()
			request.StartedAt = &startedAt
		}
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
		ViewingMethod: round.ViewingMethod, WatchedAt: round.CompletedAt, StartedAt: round.StartedAt,
		Version: round.Version, ProfileVersion: round.ProfileVersion,
		ParticipantIDs: append([]string{}, round.ParticipantIDs...),
	}
	if round.Rating != nil {
		rating := records.RatingToTen(*round.Rating)
		response.Rating = &rating
	}
	return response
}

func newArchivedRoundResponse(round records.WatchRound) archivedRoundResponse {
	response := archivedRoundResponse{
		RoundID: round.ID, MediaID: round.MediaID, SeasonNumber: round.SeasonNumber,
		RoundNumber: round.RoundNumber, Status: round.Status, Note: round.Note,
		ViewingMethod: round.ViewingMethod, WatchedAt: round.CompletedAt, ArchivedAt: round.ArchivedAt,
	}
	if round.Rating != nil {
		rating := records.RatingToTen(*round.Rating)
		response.Rating = &rating
	}
	return response
}
