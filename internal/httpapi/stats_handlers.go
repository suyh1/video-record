package httpapi

import (
	"errors"
	"net/http"
	"strings"

	statsdomain "video-record/internal/stats"
)

type statsResponse struct {
	TotalWatches   int             `json:"totalWatches"`
	UniqueMedia    int             `json:"uniqueMedia"`
	TotalMinutes   int             `json:"totalMinutes"`
	RepeatWatches  int             `json:"repeatWatches"`
	Monthly        []statsPointDTO `json:"monthly"`
	Yearly         []statsPointDTO `json:"yearly"`
	Genres         []statsPointDTO `json:"genres"`
	Ratings        []statsPointDTO `json:"ratings"`
	Tags           []statsPointDTO `json:"tags"`
	ViewingMethods []statsPointDTO `json:"viewingMethods"`
}

type statsPointDTO struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type statsHandlers struct {
	service *statsdomain.Service
}

func (handlers statsHandlers) summary(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	timezone := strings.TrimSpace(r.URL.Query().Get("timezone"))
	if timezone == "" {
		timezone = "UTC"
	}
	summary, err := handlers.service.Summary(r.Context(), identity.User.ID, timezone)
	if errors.Is(err, statsdomain.ErrInvalidStatsQuery) {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_stats_query")
		return
	}
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, newStatsResponse(summary))
}

func newStatsResponse(summary statsdomain.Summary) statsResponse {
	return statsResponse{
		TotalWatches: summary.TotalWatches, UniqueMedia: summary.UniqueMedia,
		TotalMinutes: summary.TotalMinutes, RepeatWatches: summary.RepeatWatches,
		Monthly: newStatsPoints(summary.Monthly), Yearly: newStatsPoints(summary.Yearly),
		Genres: newStatsPoints(summary.Genres), Ratings: newStatsPoints(summary.Ratings),
		Tags: newStatsPoints(summary.Tags), ViewingMethods: newStatsPoints(summary.ViewingMethods),
	}
}

func newStatsPoints(points []statsdomain.Point) []statsPointDTO {
	response := make([]statsPointDTO, 0, len(points))
	for _, point := range points {
		response = append(response, statsPointDTO{Label: point.Label, Value: point.Value})
	}
	return response
}
