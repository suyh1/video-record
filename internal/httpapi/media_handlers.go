package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations/tmdb"
	"video-record/internal/media"
)

type mediaHandlers struct {
	service *media.Service
	tmdb    *tmdb.Client
	now     func() time.Time
}

type customMediaRequest struct {
	MediaType media.MediaType `json:"mediaType"`
	Title     string          `json:"title"`
	Overview  string          `json:"overview"`
	Year      string          `json:"year"`
}

type mediaItemResponse struct {
	ID               string          `json:"id"`
	TMDBID           *int            `json:"tmdbId"`
	MediaType        media.MediaType `json:"mediaType"`
	Title            string          `json:"title"`
	Overview         string          `json:"overview"`
	ExternalTitle    string          `json:"externalTitle"`
	ExternalOverview string          `json:"externalOverview"`
	OriginalTitle    string          `json:"originalTitle"`
	ReleaseDate      string          `json:"releaseDate"`
	PosterPath       string          `json:"posterPath"`
	BackdropPath     string          `json:"backdropPath"`
	RuntimeMinutes   int             `json:"runtimeMinutes"`
	Genres           []string        `json:"genres"`
}

func (handlers mediaHandlers) get(w http.ResponseWriter, r *http.Request) {
	item, err := handlers.service.FindByID(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeProblem(w, r, http.StatusNotFound, "Not Found", "media_not_found")
		return
	}
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, handlers.newMediaItemResponse(item))
}

func (handlers mediaHandlers) createFromTMDB(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := handlers.tmdbSnapshot(w, r)
	if !ok {
		return
	}
	item, err := handlers.service.UpsertExternal(r.Context(), snapshot)
	if err != nil {
		writeMediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, handlers.newMediaItemResponse(item))
}

func (handlers mediaHandlers) createCustom(w http.ResponseWriter, r *http.Request) {
	var request customMediaRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	item, err := handlers.service.CreateCustom(r.Context(), media.CreateCustomInput{
		MediaType: request.MediaType,
		Title:     request.Title,
		Overview:  request.Overview,
		Year:      request.Year,
	})
	if err != nil {
		writeMediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, handlers.newMediaItemResponse(item))
}

func (handlers mediaHandlers) linkTMDB(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := handlers.tmdbSnapshot(w, r)
	if !ok {
		return
	}
	item, err := handlers.service.LinkExternal(r.Context(), chi.URLParam(r, "id"), snapshot)
	if err != nil {
		writeMediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, handlers.newMediaItemResponse(item))
}

func (handlers mediaHandlers) tmdbSnapshot(w http.ResponseWriter, r *http.Request) (media.ExternalSnapshot, bool) {
	externalID, ok := positiveURLInt(w, r, "externalID")
	if !ok {
		return media.ExternalSnapshot{}, false
	}
	mediaType := media.MediaType(chi.URLParam(r, "mediaType"))
	snapshot := media.ExternalSnapshot{
		Source:    "tmdb",
		SourceID:  strconv.Itoa(externalID),
		MediaType: mediaType,
	}
	switch mediaType {
	case media.MediaTypeMovie:
		item, err := handlers.tmdb.MovieDetails(r.Context(), externalID, "zh-CN")
		if err != nil {
			writeTMDBError(w, r, err)
			return media.ExternalSnapshot{}, false
		}
		snapshot.Title = item.Title
		snapshot.OriginalTitle = item.OriginalTitle
		snapshot.ReleaseDate = item.ReleaseDate
		snapshot.Overview = item.Overview
		snapshot.PosterPath = item.PosterPath
		snapshot.BackdropPath = item.BackdropPath
		snapshot.RuntimeMinutes = item.Runtime
		snapshot.Genres = externalGenres(item.Genres)
	case media.MediaTypeTV:
		item, err := handlers.tmdb.TVDetails(r.Context(), externalID, "zh-CN")
		if err != nil {
			writeTMDBError(w, r, err)
			return media.ExternalSnapshot{}, false
		}
		snapshot.Title = item.Name
		snapshot.OriginalTitle = item.OriginalName
		snapshot.ReleaseDate = item.FirstAirDate
		snapshot.Overview = item.Overview
		snapshot.PosterPath = item.PosterPath
		snapshot.BackdropPath = item.BackdropPath
		snapshot.Genres = externalGenres(item.Genres)
	default:
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_media_type")
		return media.ExternalSnapshot{}, false
	}
	return snapshot, true
}

func (handlers mediaHandlers) newMediaItemResponse(item media.Item) mediaItemResponse {
	return mediaItemResponse{
		ID: item.ID, TMDBID: item.TMDBID, MediaType: item.MediaType, Title: item.Title, Overview: item.Overview,
		ExternalTitle: item.ExternalTitle, ExternalOverview: item.ExternalOverview,
		OriginalTitle: item.OriginalTitle, ReleaseDate: item.ReleaseDate,
		PosterPath: sourceAwareMediaImageURL(
			handlers.tmdb, "w342", item.PosterPath, item.TMDBID, handlerTime(handlers.now),
		),
		BackdropPath: sourceAwareMediaImageURL(
			handlers.tmdb, "w1280", item.BackdropPath, item.TMDBID, handlerTime(handlers.now),
		),
		RuntimeMinutes: item.RuntimeMinutes, Genres: item.Genres,
	}
}

func externalGenres(genres []tmdb.Genre) []media.ExternalGenre {
	result := make([]media.ExternalGenre, 0, len(genres))
	for _, genre := range genres {
		result = append(result, media.ExternalGenre{ID: strconv.Itoa(genre.ID), Name: genre.Name})
	}
	return result
}

func writeMediaError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, media.ErrInvalidMedia), errors.Is(err, media.ErrMediaTypeMismatch):
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_media")
	case errors.Is(err, media.ErrExternalIdentityConflict):
		writeProblem(w, r, http.StatusConflict, "Conflict", "external_identity_conflict")
	case errors.Is(err, sql.ErrNoRows):
		writeProblem(w, r, http.StatusNotFound, "Not Found", "media_not_found")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}
