package httpapi

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"video-record/internal/integrations/tmdb"
)

const (
	publicTMDBHighlightLimit   = 10
	publicTMDBImageSize        = "w1280"
	publicTMDBImageTTL         = 24 * time.Hour
	publicTMDBImageConcurrency = 4
)

var (
	publicTMDBImageSizes = map[string]struct{}{
		"w300": {}, "w342": {}, "w780": {}, "w1280": {},
	}
	publicTMDBImageFilename = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(?:jpg|jpeg|png|webp)$`)
)

type publicTMDBHandlers struct {
	client     *tmdb.Client
	now        func() time.Time
	imageSlots chan struct{}
}

type publicTMDBHighlightsResponse struct {
	Items []publicTMDBHighlight `json:"items"`
}

type publicTMDBHighlight struct {
	ID            int    `json:"id"`
	MediaType     string `json:"mediaType"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle"`
	Year          string `json:"year"`
	Overview      string `json:"overview"`
	BackdropURL   string `json:"backdropURL"`
}

type publicTMDBPopularResult struct {
	mediaType string
	response  tmdb.PopularResponse
	err       error
}

func (handlers publicTMDBHandlers) highlights(w http.ResponseWriter, r *http.Request) {
	results := make(chan publicTMDBPopularResult, 2)
	for _, mediaType := range []string{"movie", "tv"} {
		mediaType := mediaType
		go func() {
			response, err := handlers.client.Popular(r.Context(), mediaType, "zh-CN")
			results <- publicTMDBPopularResult{mediaType: mediaType, response: response, err: err}
		}()
	}

	feeds := make(map[string]tmdb.PopularResponse, 2)
	errorsByType := make(map[string]error, 2)
	for range 2 {
		result := <-results
		feeds[result.mediaType] = result.response
		errorsByType[result.mediaType] = result.err
	}
	for _, mediaType := range []string{"movie", "tv"} {
		if errorsByType[mediaType] != nil {
			writeTMDBError(w, r, errorsByType[mediaType])
			return
		}
	}

	now := handlerTime(handlers.now)
	movies := handlers.mapHighlights(feeds["movie"].Results, "movie", now)
	shows := handlers.mapHighlights(feeds["tv"].Results, "tv", now)
	items := make([]publicTMDBHighlight, 0, min(publicTMDBHighlightLimit, len(movies)+len(shows)))
	for movieIndex, showIndex := 0, 0; len(items) < publicTMDBHighlightLimit &&
		(movieIndex < len(movies) || showIndex < len(shows)); {
		if movieIndex < len(movies) {
			items = append(items, movies[movieIndex])
			movieIndex++
		}
		if len(items) == publicTMDBHighlightLimit {
			break
		}
		if showIndex < len(shows) {
			items = append(items, shows[showIndex])
			showIndex++
		}
	}
	writeJSON(w, http.StatusOK, publicTMDBHighlightsResponse{Items: items})
}

func (handlers publicTMDBHandlers) mapHighlights(
	items []tmdb.PopularItem,
	mediaType string,
	now time.Time,
) []publicTMDBHighlight {
	highlights := make([]publicTMDBHighlight, 0, len(items))
	for _, item := range items {
		if item.BackdropPath == "" {
			continue
		}
		backdropURL := signedTMDBImageURL(handlers.client, publicTMDBImageSize, item.BackdropPath, now)
		if backdropURL == "" {
			continue
		}
		title, originalTitle, date := item.Title, item.OriginalTitle, item.ReleaseDate
		if mediaType == "tv" {
			title, originalTitle, date = item.Name, item.OriginalName, item.FirstAirDate
		}
		highlights = append(highlights, publicTMDBHighlight{
			ID:            item.ID,
			MediaType:     mediaType,
			Title:         title,
			OriginalTitle: originalTitle,
			Year:          yearFromDate(date),
			Overview:      item.Overview,
			BackdropURL:   backdropURL,
		})
	}
	return highlights
}

func (handlers publicTMDBHandlers) image(w http.ResponseWriter, r *http.Request) {
	size := chi.URLParam(r, "size")
	filename := chi.URLParam(r, "filename")
	expiresValue := r.URL.Query().Get("expires")
	signature := r.URL.Query().Get("signature")
	expiresUnix, err := strconv.ParseInt(expiresValue, 10, 64)
	if expiresValue == "" || signature == "" || err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_image_parameters")
		return
	}
	if !validPublicTMDBImagePath(size, filename) {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_image_request")
		return
	}
	expires := time.Unix(expiresUnix, 0)
	if !expires.After(handlers.now()) {
		writeProblem(w, r, http.StatusGone, "Gone", "image_url_expired")
		return
	}
	imagePath := "/" + filename
	if !handlers.client.VerifyImage(size, imagePath, expires, signature) {
		if !expires.After(handlers.now()) {
			writeProblem(w, r, http.StatusGone, "Gone", "image_url_expired")
			return
		}
		writeProblem(w, r, http.StatusForbidden, "Forbidden", "invalid_image_signature")
		return
	}
	if !handlers.acquireImageSlot(w, r) {
		return
	}
	defer func() { <-handlers.imageSlots }()
	asset, err := handlers.client.Image(r.Context(), size, imagePath)
	if err != nil {
		writeTMDBError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(asset.Contents)
}

func (handlers publicTMDBHandlers) acquireImageSlot(w http.ResponseWriter, r *http.Request) bool {
	select {
	case <-r.Context().Done():
		return false
	default:
	}
	select {
	case <-r.Context().Done():
		return false
	case handlers.imageSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "1")
		writeProblem(w, r, http.StatusTooManyRequests, "Too Many Requests", "tmdb_image_busy")
		return false
	}
}

func buildPublicTMDBImageURL(size, imagePath string, expires time.Time, signature string) string {
	query := url.Values{
		"expires":   {strconv.FormatInt(expires.Unix(), 10)},
		"signature": {signature},
	}
	return "/api/v1/public/tmdb/images/" + size + "/" +
		strings.TrimPrefix(imagePath, "/") + "?" + query.Encode()
}

func validPublicTMDBImagePath(size, filename string) bool {
	_, sizeAllowed := publicTMDBImageSizes[size]
	return sizeAllowed && publicTMDBImageFilename.MatchString(filename)
}
