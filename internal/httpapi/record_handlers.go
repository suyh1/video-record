package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"video-record/internal/records"
)

type recordHandlers struct {
	service *records.Service
}

type updateRecordRequest struct {
	Status    records.Status
	Rating    *float64
	RatingSet bool
	Note      *string
	NoteSet   bool
}

type recordResponse struct {
	MediaID string         `json:"mediaId"`
	Status  records.Status `json:"status"`
	Rating  *float64       `json:"rating"`
	Note    *string        `json:"note"`
	Version int            `json:"version"`
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

type collectionResponse struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func (handlers recordHandlers) updateState(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	expectedVersion, ok := parseIfMatch(w, r)
	if !ok {
		return
	}
	request, err := decodeUpdateRecordRequest(r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	var rating *int
	if request.Rating != nil {
		converted, err := records.RatingFromTen(*request.Rating)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_rating")
			return
		}
		rating = &converted
	}
	state, err := handlers.service.UpdateState(r.Context(), records.UpdateStateInput{
		UserID: identity.User.ID, MediaID: chi.URLParam(r, "mediaID"),
		Status: request.Status, Rating: rating, RatingSet: request.RatingSet,
		Note: request.Note, NoteSet: request.NoteSet,
		Source: records.SourceManual, ExpectedVersion: expectedVersion,
	})
	if err != nil {
		writeRecordError(w, r, err, state.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(state.Version))
	writeJSON(w, http.StatusOK, newRecordResponse(state))
}

func decodeUpdateRecordRequest(r *http.Request) (updateRecordRequest, error) {
	var raw struct {
		Status records.Status  `json:"status"`
		Rating json.RawMessage `json:"rating"`
		Note   json.RawMessage `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return updateRecordRequest{}, err
	}
	request := updateRecordRequest{Status: raw.Status}
	if raw.Rating != nil {
		request.RatingSet = true
		if string(raw.Rating) != "null" {
			if err := json.Unmarshal(raw.Rating, &request.Rating); err != nil {
				return updateRecordRequest{}, err
			}
		}
	}
	if raw.Note != nil {
		request.NoteSet = true
		if string(raw.Note) != "null" {
			if err := json.Unmarshal(raw.Note, &request.Note); err != nil {
				return updateRecordRequest{}, err
			}
		}
	}
	return request, nil
}

func (handlers recordHandlers) setTags(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request tagsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	if err := handlers.service.SetTags(r.Context(), identity.User.ID, chi.URLParam(r, "mediaID"), request.Tags); err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func newRecordResponse(state records.State) recordResponse {
	response := recordResponse{
		MediaID: state.MediaID, Status: state.Status, Note: state.Note, Version: state.Version,
	}
	if state.Rating != nil {
		value := records.RatingToTen(*state.Rating)
		response.Rating = &value
	}
	return response
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
	case errors.Is(err, records.ErrInvalidRecord), errors.Is(err, records.ErrInvalidStatus):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_record")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}
