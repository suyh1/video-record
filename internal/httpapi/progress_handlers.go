package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/records"
)

type episodeProgressRequest struct {
	Action           records.EpisodeProgressAction `json:"action"`
	EpisodeID        string                        `json:"episodeId"`
	ThroughEpisodeID string                        `json:"throughEpisodeId"`
	SeasonID         string                        `json:"seasonId"`
	WatchedAt        time.Time                     `json:"watchedAt"`
	ExpectedVersion  int                           `json:"expectedVersion"`
}

type episodeProgressResponse struct {
	MediaID         string                `json:"mediaId"`
	Status          records.Status        `json:"status"`
	Version         int                   `json:"version"`
	WatchedEpisodes int                   `json:"watchedEpisodes"`
	TotalEpisodes   int                   `json:"totalEpisodes"`
	LastWatched     *episodeProgressItem  `json:"lastWatched"`
	NextEpisode     *episodeProgressItem  `json:"nextEpisode"`
	Episodes        []episodeProgressItem `json:"episodes"`
}

type episodeProgressItem struct {
	ID             string     `json:"id"`
	SeasonID       string     `json:"seasonId"`
	SeasonNumber   int        `json:"seasonNumber"`
	EpisodeNumber  int        `json:"episodeNumber"`
	AbsoluteNumber int        `json:"absoluteNumber"`
	Name           string     `json:"name"`
	Watched        bool       `json:"watched"`
	WatchedAt      *time.Time `json:"watchedAt"`
}

func (handlers recordHandlers) episodeProgress(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	progress, err := handlers.service.EpisodeProgress(r.Context(), identity.User.ID, chi.URLParam(r, "mediaID"))
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	w.Header().Set("ETag", quotedVersion(progress.Version))
	writeJSON(w, http.StatusOK, newEpisodeProgressResponse(progress))
}

func (handlers recordHandlers) updateEpisodeProgress(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	var request episodeProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	progress, err := handlers.service.UpdateEpisodeProgress(r.Context(), records.EpisodeProgressInput{
		UserID: identity.User.ID, MediaID: chi.URLParam(r, "mediaID"),
		Action: request.Action, EpisodeID: request.EpisodeID,
		ThroughEpisodeID: request.ThroughEpisodeID, SeasonID: request.SeasonID,
		WatchedAt: request.WatchedAt, Source: records.SourceManual,
		ExpectedVersion: request.ExpectedVersion,
	})
	if err != nil {
		writeRecordError(w, r, err, progress.Version)
		return
	}
	w.Header().Set("ETag", quotedVersion(progress.Version))
	writeJSON(w, http.StatusOK, newEpisodeProgressResponse(progress))
}

func newEpisodeProgressResponse(progress records.SeriesProgress) episodeProgressResponse {
	response := episodeProgressResponse{
		MediaID: progress.MediaID, Status: progress.Status, Version: progress.Version,
		WatchedEpisodes: progress.WatchedEpisodes, TotalEpisodes: progress.TotalEpisodes,
		Episodes: make([]episodeProgressItem, 0, len(progress.Episodes)),
	}
	for _, episode := range progress.Episodes {
		response.Episodes = append(response.Episodes, newEpisodeProgressItem(episode))
	}
	if progress.LastWatched != nil {
		item := newEpisodeProgressItem(*progress.LastWatched)
		response.LastWatched = &item
	}
	if progress.NextEpisode != nil {
		item := newEpisodeProgressItem(*progress.NextEpisode)
		response.NextEpisode = &item
	}
	return response
}

func newEpisodeProgressItem(episode records.Episode) episodeProgressItem {
	return episodeProgressItem{
		ID: episode.ID, SeasonID: episode.SeasonID,
		SeasonNumber: episode.SeasonNumber, EpisodeNumber: episode.EpisodeNumber,
		AbsoluteNumber: episode.AbsoluteNumber, Name: episode.Name,
		Watched: episode.Watched, WatchedAt: episode.WatchedAt,
	}
}
