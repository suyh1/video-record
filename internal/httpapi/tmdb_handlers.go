package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations/tmdb"
)

type tmdbHandlers struct {
	client *tmdb.Client
	now    func() time.Time
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
	ID            int      `json:"id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle"`
	ReleaseDate   string   `json:"releaseDate"`
	PosterPath    string   `json:"posterPath"`
	BackdropPath  string   `json:"backdropPath"`
	Overview      string   `json:"overview"`
	Runtime       int      `json:"runtime"`
	Genres        []string `json:"genres"`
}

type tmdbTVResponse struct {
	ID             int                         `json:"id"`
	Name           string                      `json:"name"`
	OriginalName   string                      `json:"originalName"`
	FirstAirDate   string                      `json:"firstAirDate"`
	PosterPath     string                      `json:"posterPath"`
	BackdropPath   string                      `json:"backdropPath"`
	Overview       string                      `json:"overview"`
	NumberSeasons  int                         `json:"numberOfSeasons"`
	NumberEpisodes int                         `json:"numberOfEpisodes"`
	EpisodeRuntime []int                       `json:"episodeRuntime"`
	Genres         []string                    `json:"genres"`
	Seasons        []tmdbSeasonSummaryResponse `json:"seasons"`
}

type tmdbSeasonSummaryResponse struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"posterPath"`
	AirDate      string `json:"airDate"`
	SeasonNumber int    `json:"seasonNumber"`
	EpisodeCount int    `json:"episodeCount"`
}

type tmdbSeasonResponse struct {
	ID           int                   `json:"id"`
	Name         string                `json:"name"`
	Overview     string                `json:"overview"`
	PosterPath   string                `json:"posterPath"`
	AirDate      string                `json:"airDate"`
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
	StillPath     string `json:"stillPath"`
}

type tmdbCreditsResponse struct {
	Cast []tmdbCastResponse `json:"cast"`
}

type tmdbCastResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profilePath"`
	Order       int    `json:"order"`
}

func (handlers tmdbHandlers) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"configured": handlers.client.Configured()})
}

func (handlers tmdbHandlers) connectivity(w http.ResponseWriter, r *http.Request) {
	if err := handlers.client.TestConnectivity(r.Context()); err != nil {
		writeTMDBError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
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
			PosterPath:    signedTMDBImageURL(handlers.client, "w342", result.PosterPath, handlerTime(handlers.now)),
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
		ReleaseDate:  item.ReleaseDate,
		PosterPath:   signedTMDBImageURL(handlers.client, "w342", item.PosterPath, handlerTime(handlers.now)),
		BackdropPath: signedTMDBImageURL(handlers.client, "w1280", item.BackdropPath, handlerTime(handlers.now)),
		Overview:     item.Overview, Runtime: item.Runtime,
		Genres: genreNames(item.Genres),
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
	seasons := make([]tmdbSeasonSummaryResponse, 0, len(item.Seasons))
	for _, season := range item.Seasons {
		seasons = append(seasons, tmdbSeasonSummaryResponse{
			ID: season.ID, Name: season.Name, Overview: season.Overview,
			PosterPath:   signedTMDBImageURL(handlers.client, "w342", season.PosterPath, handlerTime(handlers.now)),
			AirDate:      season.AirDate,
			SeasonNumber: season.SeasonNumber, EpisodeCount: season.EpisodeCount,
		})
	}
	writeJSON(w, http.StatusOK, tmdbTVResponse{
		ID: item.ID, Name: item.Name, OriginalName: item.OriginalName,
		FirstAirDate:  item.FirstAirDate,
		PosterPath:    signedTMDBImageURL(handlers.client, "w342", item.PosterPath, handlerTime(handlers.now)),
		BackdropPath:  signedTMDBImageURL(handlers.client, "w1280", item.BackdropPath, handlerTime(handlers.now)),
		Overview:      item.Overview,
		NumberSeasons: item.NumberSeasons, NumberEpisodes: item.NumberEpisodes,
		EpisodeRuntime: item.EpisodeRunTime, Genres: genreNames(item.Genres), Seasons: seasons,
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
		episodes = append(episodes, handlers.newTMDBEpisodeResponse(episode))
	}
	writeJSON(w, http.StatusOK, tmdbSeasonResponse{
		ID: item.ID, Name: item.Name, Overview: item.Overview,
		PosterPath:   signedTMDBImageURL(handlers.client, "w342", item.PosterPath, handlerTime(handlers.now)),
		AirDate:      item.AirDate,
		SeasonNumber: item.SeasonNumber, Episodes: episodes,
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
	writeJSON(w, http.StatusOK, handlers.newTMDBEpisodeResponse(item))
}

func (handlers tmdbHandlers) newTMDBEpisodeResponse(item tmdb.EpisodeDetails) tmdbEpisodeResponse {
	return tmdbEpisodeResponse{
		ID: item.ID, Name: item.Name, Overview: item.Overview, AirDate: item.AirDate,
		SeasonNumber: item.SeasonNumber, EpisodeNumber: item.EpisodeNumber,
		Runtime:   item.Runtime,
		StillPath: signedTMDBImageURL(handlers.client, "w780", item.StillPath, handlerTime(handlers.now)),
	}
}

func (handlers tmdbHandlers) credits(w http.ResponseWriter, r *http.Request) {
	mediaType := chi.URLParam(r, "mediaType")
	if mediaType != "movie" && mediaType != "tv" {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_media_type")
		return
	}
	id, ok := positiveURLInt(w, r, "id")
	if !ok {
		return
	}
	credits, err := handlers.client.Credits(r.Context(), mediaType, id, "zh-CN")
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	sort.SliceStable(credits.Cast, func(i, j int) bool { return credits.Cast[i].Order < credits.Cast[j].Order })
	if len(credits.Cast) > 20 {
		credits.Cast = credits.Cast[:20]
	}
	cast := make([]tmdbCastResponse, 0, len(credits.Cast))
	for _, member := range credits.Cast {
		cast = append(cast, tmdbCastResponse{
			ID: member.ID, Name: member.Name, Character: member.Character,
			ProfilePath: signedTMDBImageURL(handlers.client, "w300", member.ProfilePath, handlerTime(handlers.now)),
			Order:       member.Order,
		})
	}
	writeJSON(w, http.StatusOK, tmdbCreditsResponse{Cast: cast})
}

func signedTMDBImageURL(client *tmdb.Client, size, imagePath string, now time.Time) string {
	if imagePath == "" || client == nil {
		return ""
	}
	expires := now.Add(publicTMDBImageTTL)
	signature, err := client.SignImage(size, imagePath, expires)
	if err != nil {
		return ""
	}
	return buildPublicTMDBImageURL(size, imagePath, expires, signature)
}

func sourceAwareMediaImageURL(
	client *tmdb.Client,
	size, imagePath string,
	tmdbID *int,
	now time.Time,
) string {
	if tmdbID != nil {
		return signedTMDBImageURL(client, size, imagePath, now)
	}
	parsed, err := url.Parse(imagePath)
	if err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) &&
		parsed.Host != "" {
		if strings.EqualFold(parsed.Hostname(), "image.tmdb.org") {
			return ""
		}
		return imagePath
	}
	return ""
}

func handlerTime(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}

func genreNames(genres []tmdb.Genre) []string {
	names := make([]string, 0, len(genres))
	for _, genre := range genres {
		names = append(names, genre.Name)
	}
	return names
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
	case errors.Is(err, tmdb.ErrUnauthorized):
		writeProblem(w, r, http.StatusBadGateway, "Bad Gateway", "tmdb_unauthorized")
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
