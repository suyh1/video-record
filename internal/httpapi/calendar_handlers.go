package httpapi

import (
	"net/http"
	"strings"
	"time"

	"video-record/internal/records"
)

type calendarResponse struct {
	Year     int                     `json:"year"`
	Month    int                     `json:"month"`
	Timezone string                  `json:"timezone"`
	Events   []calendarEventResponse `json:"events"`
}

type calendarEventResponse struct {
	ID             string         `json:"id"`
	MediaID        string         `json:"mediaId"`
	MediaType      string         `json:"mediaType"`
	Title          string         `json:"title"`
	EpisodeID      *string        `json:"episodeId"`
	SeasonNumber   *int           `json:"seasonNumber"`
	EpisodeNumber  *int           `json:"episodeNumber"`
	AbsoluteNumber *int           `json:"absoluteNumber"`
	WatchedAt      time.Time      `json:"watchedAt"`
	LocalDate      string         `json:"localDate"`
	ViewingMethod  *string        `json:"viewingMethod"`
	Participants   []string       `json:"participants"`
	Status         records.Status `json:"status"`
}

func (handlers recordHandlers) calendar(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	month, err := time.Parse("2006-01", strings.TrimSpace(r.URL.Query().Get("month")))
	if err != nil {
		writeRecordError(w, r, records.ErrInvalidCalendarQuery, 0)
		return
	}
	filter := records.CalendarFilter(strings.TrimSpace(r.URL.Query().Get("filter")))
	if filter == "" {
		filter = records.CalendarFilterAll
	}
	calendar, err := handlers.service.CalendarMonth(r.Context(), records.CalendarMonthInput{
		UserID: identity.User.ID, Year: month.Year(), Month: month.Month(),
		Timezone: r.URL.Query().Get("timezone"), Filter: filter,
	})
	if err != nil {
		writeRecordError(w, r, err, 0)
		return
	}
	writeJSON(w, http.StatusOK, newCalendarResponse(calendar))
}

func newCalendarResponse(calendar records.CalendarMonth) calendarResponse {
	response := calendarResponse{
		Year: calendar.Year, Month: int(calendar.Month), Timezone: calendar.Timezone,
		Events: make([]calendarEventResponse, 0, len(calendar.Events)),
	}
	for _, event := range calendar.Events {
		response.Events = append(response.Events, calendarEventResponse{
			ID: event.ID, MediaID: event.MediaID, MediaType: event.MediaType, Title: event.Title,
			EpisodeID: event.EpisodeID, SeasonNumber: event.SeasonNumber,
			EpisodeNumber: event.EpisodeNumber, AbsoluteNumber: event.AbsoluteNumber,
			WatchedAt: event.WatchedAt, LocalDate: event.LocalDate,
			ViewingMethod: event.ViewingMethod, Participants: event.Participants, Status: event.Status,
		})
	}
	return response
}
