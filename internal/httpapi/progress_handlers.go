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

type episodeProgressRequest struct {
	Action           records.EpisodeProgressAction `json:"action"`
	EpisodeID        string                        `json:"episodeId"`
	ThroughEpisodeID string                        `json:"throughEpisodeId"`
	SeasonID         string                        `json:"seasonId"`
	WatchedAt        time.Time                     `json:"watchedAt"`
	ExpectedVersion  int                           `json:"expectedVersion"`
	EpisodeRefs      []episodeReferenceRequest     `json:"episodeRefs"`
	TotalEpisodes    int                           `json:"totalEpisodes"`
}

type episodeReferenceRequest struct {
	SourceID       string `json:"sourceId"`
	SeasonNumber   int    `json:"seasonNumber"`
	EpisodeNumber  int    `json:"episodeNumber"`
	AbsoluteNumber int    `json:"absoluteNumber"`
}

type episodeProgressResponse struct {
	RoundID         string                `json:"roundId"`
	MediaID         string                `json:"mediaId"`
	SeasonNumber    int                   `json:"seasonNumber"`
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
	SourceID       string     `json:"sourceId"`
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
	seasonNumber, ok := episodeSeasonNumberFromRequest(w, r)
	if !ok {
		return
	}
	progress, err := handlers.service.EpisodeProgress(
		r.Context(), identity.User.ID, chi.URLParam(r, "mediaID"), seasonNumber,
	)
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
	seasonNumber, ok := episodeSeasonNumberFromRequest(w, r)
	if !ok {
		return
	}
	var request episodeProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_request")
		return
	}
	references := make([]records.EpisodeReference, 0, len(request.EpisodeRefs))
	for _, episode := range request.EpisodeRefs {
		references = append(references, records.EpisodeReference{
			SourceID: episode.SourceID, SeasonNumber: episode.SeasonNumber,
			EpisodeNumber: episode.EpisodeNumber, AbsoluteNumber: episode.AbsoluteNumber,
		})
	}
	progress, err := handlers.service.UpdateEpisodeProgress(r.Context(), records.EpisodeProgressInput{
		UserID: identity.User.ID, MediaID: chi.URLParam(r, "mediaID"), SeasonNumber: seasonNumber,
		Action: request.Action, EpisodeID: request.EpisodeID,
		ThroughEpisodeID: request.ThroughEpisodeID, SeasonID: request.SeasonID,
		WatchedAt: request.WatchedAt, Source: records.SourceManual,
		ExpectedVersion: request.ExpectedVersion, EpisodeRefs: references,
		TotalEpisodes: request.TotalEpisodes,
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
		RoundID: progress.RoundID, MediaID: progress.MediaID, SeasonNumber: progress.SeasonNumber,
		Status: progress.Status, Version: progress.Version,
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

func episodeSeasonNumberFromRequest(w http.ResponseWriter, r *http.Request) (int, bool) {
	values, exists := r.URL.Query()["seasonNumber"]
	if !exists || len(values) != 1 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_episode_progress")
		return 0, false
	}
	seasonNumber, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil || seasonNumber < 1 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_episode_progress")
		return 0, false
	}
	return seasonNumber, true
}

func newEpisodeProgressItem(episode records.Episode) episodeProgressItem {
	return episodeProgressItem{
		ID: episode.ID, SourceID: episode.SourceID, SeasonID: episode.SeasonID,
		SeasonNumber: episode.SeasonNumber, EpisodeNumber: episode.EpisodeNumber,
		AbsoluteNumber: episode.AbsoluteNumber, Name: episode.Name,
		Watched: episode.Watched, WatchedAt: episode.WatchedAt,
	}
}
