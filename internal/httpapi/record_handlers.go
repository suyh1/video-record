package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations/tmdb"
	"video-record/internal/records"
)

type recordHandlers struct {
	service     *records.Service
	idempotency *idempotencyMiddleware
	tmdb        *tmdb.Client
	now         func() time.Time
}

type recordResponse struct {
	MediaID       string         `json:"mediaId"`
	Status        records.Status `json:"status"`
	Rating        *float64       `json:"rating"`
	Note          *string        `json:"note"`
	WatchedAt     *time.Time     `json:"watchedAt"`
	ViewingMethod *string        `json:"viewingMethod"`
	Version       int            `json:"version"`
}

type tagsRequest struct {
	Tags []string `json:"tags"`
}

type collectionRequest struct {
	Name string `json:"name"`
}

type collectionItemRequest struct {
	MediaID string `json:"mediaId"`
}

type collectionItemsRequest struct {
	MediaIDs []string `json:"mediaIds"`
}

type collectionResponse struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func (handlers recordHandlers) getRecord(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	mediaID := chi.URLParam(r, "mediaID")
	state, _, err := handlers.service.State(r.Context(), identity.User.ID, mediaID)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.Header().Set("ETag", quotedVersion(state.Version))
	writeJSON(w, http.StatusOK, newRecordResponse(state, handlers.latestWatchEvent(r, mediaID)))
}

func (handlers recordHandlers) library(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	status := records.Status(strings.TrimSpace(r.URL.Query().Get("status")))
	items, err := handlers.service.Library(r.Context(), identity.User.ID, status)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": handlers.newCatalogResponses(items), "nextCursor": nil})
}

func (handlers recordHandlers) localSearch(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	items, err := handlers.service.SearchMedia(r.Context(), identity.User.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": handlers.newCatalogResponses(items)})
}

func (handlers recordHandlers) setTags(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	expectedVersion, ok := parseIfMatch(w, r)
	if !ok {
		return
	}
	var request tagsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	state, err := handlers.service.SetTagsVersioned(
		r.Context(), identity.User.ID, chi.URLParam(r, "mediaID"), request.Tags, expectedVersion,
	)
	if err != nil {
		writeRecordError(w, r, err, state.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(state.Version))
	w.WriteHeader(http.StatusNoContent)
}

func (handlers recordHandlers) tags(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	mediaID := chi.URLParam(r, "mediaID")
	state, _, err := handlers.service.State(r.Context(), identity.User.ID, mediaID)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	tags, err := handlers.service.Tags(r.Context(), identity.User.ID, mediaID)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.Header().Set("ETag", quotedVersion(state.Version))
	writeJSON(w, http.StatusOK, tagsRequest{Tags: tags})
}

func (handlers recordHandlers) createCollection(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request collectionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	collection, err := handlers.service.CreateCollection(r.Context(), identity.User.ID, request.Name)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	writeJSON(w, http.StatusCreated, newCollectionResponse(collection))
}

func (handlers recordHandlers) addCollectionItem(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request collectionItemRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	err := handlers.service.AddCollectionItem(
		r.Context(), identity.User.ID, chi.URLParam(r, "collectionID"), request.MediaID,
	)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handlers recordHandlers) replaceCollectionItems(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request collectionItemsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	if err := handlers.service.ReplaceCollectionItems(
		r.Context(), identity.User.ID, chi.URLParam(r, "collectionID"), request.MediaIDs,
	); err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (handlers recordHandlers) collections(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	collections, err := handlers.service.Collections(r.Context(), identity.User.ID)
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	response := make([]collectionResponse, 0, len(collections))
	for _, collection := range collections {
		response = append(response, newCollectionResponse(collection))
	}
	writeJSON(w, http.StatusOK, response)
}

func newRecordResponse(state records.State, event *records.WatchEvent) recordResponse {
	response := recordResponse{
		MediaID: state.MediaID, Status: state.Status, Note: state.Note, Version: state.Version,
		WatchedAt: state.CompletedAt,
	}
	if state.Rating != nil {
		value := records.RatingToTen(*state.Rating)
		response.Rating = &value
	}
	if event != nil {
		response.WatchedAt = &event.WatchedAt
		if event.ViewingMethod != "" {
			value := event.ViewingMethod
			response.ViewingMethod = &value
		}
	}
	return response
}

type catalogItemResponse struct {
	ID            string         `json:"id"`
	TMDBID        *int           `json:"tmdbId"`
	Source        string         `json:"source"`
	MediaType     string         `json:"mediaType"`
	Title         string         `json:"title"`
	OriginalTitle string         `json:"originalTitle"`
	Year          string         `json:"year"`
	PosterPath    *string        `json:"posterPath"`
	Status        records.Status `json:"status"`
}

func (handlers recordHandlers) newCatalogResponses(items []records.CatalogItem) []catalogItemResponse {
	response := make([]catalogItemResponse, 0, len(items))
	for _, item := range items {
		var posterPath *string
		if item.PosterPath != "" {
			value := proxiedTMDBOrCustomImageURL(handlers.tmdb, "w342", item.PosterPath, handlerTime(handlers.now))
			if value != "" {
				posterPath = &value
			}
		}
		response = append(response, catalogItemResponse{
			ID: item.ID, TMDBID: item.TMDBID, Source: "local", MediaType: item.MediaType, Title: item.Title,
			OriginalTitle: item.OriginalTitle, Year: item.Year, PosterPath: posterPath, Status: item.Status,
		})
	}
	return response
}

func (handlers recordHandlers) latestWatchEvent(r *http.Request, mediaID string) *records.WatchEvent {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		return nil
	}
	events, err := handlers.service.WatchEvents(r.Context(), identity.User.ID, mediaID)
	if err != nil || len(events) == 0 {
		return nil
	}
	return &events[len(events)-1]
}

func newCollectionResponse(collection records.Collection) collectionResponse {
	return collectionResponse{ID: collection.ID, Name: collection.Name, Items: collection.Items}
}

func parseIfMatch(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		writeProblem(w, r, http.StatusPreconditionRequired, "Precondition Required", "if_match_required")
		return 0, false
	}
	version, err := strconv.Atoi(raw[1 : len(raw)-1])
	if err != nil || version < 0 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_if_match")
		return 0, false
	}
	return version, true
}

func quotedVersion(version int) string {
	return fmt.Sprintf("\"%d\"", version)
}

func writeRecordError(w http.ResponseWriter, r *http.Request, err error, version int) {
	switch {
	case errors.Is(err, records.ErrVersionConflict):
		w.Header().Set("ETag", quotedVersion(version))
		writeProblem(w, r, http.StatusConflict, "Conflict", "version_conflict")
	case errors.Is(err, records.ErrCollectionNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "collection_not_found")
	case errors.Is(err, records.ErrInvalidRating):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_rating")
	case errors.Is(err, records.ErrInvalidRoundScope):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_round_scope")
	case errors.Is(err, records.ErrInvalidWatchedAt):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_watched_at")
	case errors.Is(err, records.ErrRoundArchived):
		writeProblem(w, r, http.StatusConflict, "Conflict", "round_archived")
	case errors.Is(err, records.ErrRoundNotCompleted):
		writeProblem(w, r, http.StatusConflict, "Conflict", "round_not_completed")
	case errors.Is(err, records.ErrRoundNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "round_not_found")
	case errors.Is(err, records.ErrInvalidRecord), errors.Is(err, records.ErrInvalidStatus):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_record")
	case errors.Is(err, records.ErrInvalidWatchEvent):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_watch_event")
	case errors.Is(err, records.ErrEpisodeNotFound):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "episode_not_found")
	case errors.Is(err, records.ErrInvalidEpisodeProgress):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_episode_progress")
	case errors.Is(err, records.ErrInvalidCalendarQuery):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_calendar_query")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}
