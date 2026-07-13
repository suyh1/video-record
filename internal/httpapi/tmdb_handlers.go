package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations/tmdb"
)

type tmdbHandlers struct {
	client *tmdb.Client
}

type tmdbSearchResponse struct {
	Page         int                `json:"page"`
	Results      []tmdbSearchResult `json:"results"`
	TotalPages   int                `json:"totalPages"`
	TotalResults int                `json:"totalResults"`
}

type tmdbSearchResult struct {
	ID            int    `json:"id"`
	MediaType     string `json:"mediaType"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle"`
	Year          string `json:"year"`
	PosterPath    string `json:"posterPath"`
	Overview      string `json:"overview"`
}

type tmdbMovieResponse struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle"`
	ReleaseDate   string `json:"releaseDate"`
	PosterPath    string `json:"posterPath"`
	BackdropPath  string `json:"backdropPath"`
	Overview      string `json:"overview"`
	Runtime       int    `json:"runtime"`
}

type tmdbTVResponse struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	OriginalName   string `json:"originalName"`
	FirstAirDate   string `json:"firstAirDate"`
	PosterPath     string `json:"posterPath"`
	BackdropPath   string `json:"backdropPath"`
	Overview       string `json:"overview"`
	NumberSeasons  int    `json:"numberOfSeasons"`
	NumberEpisodes int    `json:"numberOfEpisodes"`
}

type tmdbSeasonResponse struct {
	ID           int                   `json:"id"`
	Name         string                `json:"name"`
	SeasonNumber int                   `json:"seasonNumber"`
	Episodes     []tmdbEpisodeResponse `json:"episodes"`
}

type tmdbEpisodeResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"airDate"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Runtime       int    `json:"runtime"`
}

func (handlers tmdbHandlers) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"configured": handlers.client.Configured()})
}

func (handlers tmdbHandlers) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_query")
		return
	}
	response, err := handlers.client.Search(r.Context(), query, "zh-CN")
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	results := make([]tmdbSearchResult, 0, len(response.Results))
	for _, result := range response.Results {
		title, originalTitle, date := result.Title, result.OriginalTitle, result.ReleaseDate
		if result.MediaType == "tv" {
			title, originalTitle, date = result.Name, result.OriginalName, result.FirstAirDate
		}
		results = append(results, tmdbSearchResult{
			ID:            result.ID,
			MediaType:     result.MediaType,
			Title:         title,
			OriginalTitle: originalTitle,
			Year:          yearFromDate(date),
			PosterPath:    result.PosterPath,
			Overview:      result.Overview,
		})
	}
	writeJSON(w, http.StatusOK, tmdbSearchResponse{
		Page:         response.Page,
		Results:      results,
		TotalPages:   response.TotalPages,
		TotalResults: response.TotalResults,
	})
}

func (handlers tmdbHandlers) movie(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveURLInt(w, r, "id")
	if !ok {
		return
	}
	item, err := handlers.client.MovieDetails(r.Context(), id, "zh-CN")
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tmdbMovieResponse{
		ID: item.ID, Title: item.Title, OriginalTitle: item.OriginalTitle,
		ReleaseDate: item.ReleaseDate, PosterPath: item.PosterPath,
		BackdropPath: item.BackdropPath, Overview: item.Overview, Runtime: item.Runtime,
	})
}

func (handlers tmdbHandlers) tv(w http.ResponseWriter, r *http.Request) {
	id, ok := positiveURLInt(w, r, "id")
	if !ok {
		return
	}
	item, err := handlers.client.TVDetails(r.Context(), id, "zh-CN")
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tmdbTVResponse{
		ID: item.ID, Name: item.Name, OriginalName: item.OriginalName,
		FirstAirDate: item.FirstAirDate, PosterPath: item.PosterPath,
		BackdropPath: item.BackdropPath, Overview: item.Overview,
		NumberSeasons: item.NumberSeasons, NumberEpisodes: item.NumberEpisodes,
	})
}

func (handlers tmdbHandlers) season(w http.ResponseWriter, r *http.Request) {
	tvID, ok := positiveURLInt(w, r, "id")
	if !ok {
		return
	}
	seasonNumber, ok := positiveURLInt(w, r, "season")
	if !ok {
		return
	}
	item, err := handlers.client.SeasonDetails(r.Context(), tvID, seasonNumber, "zh-CN")
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	episodes := make([]tmdbEpisodeResponse, 0, len(item.Episodes))
	for _, episode := range item.Episodes {
		episodes = append(episodes, newTMDBEpisodeResponse(episode))
	}
	writeJSON(w, http.StatusOK, tmdbSeasonResponse{
		ID: item.ID, Name: item.Name, SeasonNumber: item.SeasonNumber, Episodes: episodes,
	})
}

func (handlers tmdbHandlers) episode(w http.ResponseWriter, r *http.Request) {
	tvID, ok := positiveURLInt(w, r, "id")
	if !ok {
		return
	}
	seasonNumber, ok := positiveURLInt(w, r, "season")
	if !ok {
		return
	}
	episodeNumber, ok := positiveURLInt(w, r, "episode")
	if !ok {
		return
	}
	item, err := handlers.client.EpisodeDetails(r.Context(), tvID, seasonNumber, episodeNumber, "zh-CN")
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newTMDBEpisodeResponse(item))
}

func newTMDBEpisodeResponse(item tmdb.EpisodeDetails) tmdbEpisodeResponse {
	return tmdbEpisodeResponse{
		ID: item.ID, Name: item.Name, Overview: item.Overview, AirDate: item.AirDate,
		SeasonNumber: item.SeasonNumber, EpisodeNumber: item.EpisodeNumber, Runtime: item.Runtime,
	}
}

func positiveURLInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	value, err := strconv.Atoi(chi.URLParam(r, name))
	if err != nil || value < 1 {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_path_parameter")
		return 0, false
	}
	return value, true
}

func writeTMDBError(w http.ResponseWriter, r *http.Request, err error) {
	var clientError *tmdb.ClientError
	switch {
	case errors.Is(err, tmdb.ErrNotConfigured):
		writeProblem(w, r, http.StatusServiceUnavailable, "Service Unavailable", "tmdb_not_configured")
	case errors.Is(err, tmdb.ErrRateLimited):
		if errors.As(err, &clientError) && clientError.RetryAfter > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", clientError.RetryAfter.Seconds()))
		}
		writeProblem(w, r, http.StatusServiceUnavailable, "Service Unavailable", "tmdb_rate_limited")
	case errors.Is(err, tmdb.ErrUpstreamTimeout):
		writeProblem(w, r, http.StatusGatewayTimeout, "Gateway Timeout", "tmdb_timeout")
	case errors.Is(err, tmdb.ErrUpstreamUnavailable):
		writeProblem(w, r, http.StatusBadGateway, "Bad Gateway", "tmdb_unavailable")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
	}
}

func yearFromDate(date string) string {
	if len(date) >= len("2006") {
		return date[:4]
	}
	return ""
}
