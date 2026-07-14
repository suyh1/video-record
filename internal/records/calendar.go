package records

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrInvalidCalendarQuery = errors.New("invalid calendar query")

type CalendarFilter string

const (
	CalendarFilterAll        CalendarFilter = "all"
	CalendarFilterCompleted  CalendarFilter = "completed"
	CalendarFilterInProgress CalendarFilter = "in_progress"
)

type CalendarMonthInput struct {
	UserID   string
	Year     int
	Month    time.Month
	Timezone string
	Filter   CalendarFilter
}

type CalendarMonth struct {
	Year     int
	Month    time.Month
	Timezone string
	Events   []CalendarEvent
}

type CalendarEvent struct {
	ID             string
	MediaID        string
	MediaType      string
	Title          string
	EpisodeID      *string
	SeasonNumber   *int
	EpisodeNumber  *int
	AbsoluteNumber *int
	WatchedAt      time.Time
	LocalDate      string
	ViewingMethod  *string
	Participants   []string
	Status         Status
}

func (service *Service) CalendarMonth(ctx context.Context, input CalendarMonthInput) (CalendarMonth, error) {
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.UserID == "" || input.Year < 1 || input.Year > 9999 || input.Month < time.January || input.Month > time.December ||
		(input.Filter != CalendarFilterAll && input.Filter != CalendarFilterCompleted && input.Filter != CalendarFilterInProgress) {
		return CalendarMonth{}, ErrInvalidCalendarQuery
	}
	location, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return CalendarMonth{}, ErrInvalidCalendarQuery
	}
	start := time.Date(input.Year, input.Month, 1, 0, 0, 0, 0, location)
	end := start.AddDate(0, 1, 0)
	events, err := service.repository.CalendarEvents(ctx, input.UserID, start.UTC(), end.UTC(), input.Filter)
	if err != nil {
		return CalendarMonth{}, err
	}
	for index := range events {
		events[index].LocalDate = events[index].WatchedAt.In(location).Format(time.DateOnly)
	}
	return CalendarMonth{
		Year: input.Year, Month: input.Month, Timezone: input.Timezone, Events: events,
	}, nil
}

func (repository *SQLiteRepository) CalendarEvents(
	ctx context.Context,
	userID string,
	start, end time.Time,
	filter CalendarFilter,
) ([]CalendarEvent, error) {
	query := `
		SELECT we.id, we.media_id, media.media_type,
		       COALESCE(media.custom_title, media.external_title),
		       we.episode_id, season.season_number, episode.episode_number,
		       CASE WHEN episode.id IS NULL THEN NULL ELSE (
		           SELECT COUNT(*)
		           FROM episodes counted_episode
		           JOIN seasons counted_season ON counted_season.id = counted_episode.season_id
		           WHERE counted_season.media_id = we.media_id
		             AND (
		               counted_season.season_number < season.season_number OR
		               (counted_season.season_number = season.season_number AND counted_episode.episode_number <= episode.episode_number)
		             )
		       ) END,
		       we.watched_at, we.viewing_method, round.status, participant_user.username
		FROM watch_events we
		JOIN watch_rounds round ON round.id = we.round_id
		JOIN watch_event_participants viewer
		  ON viewer.event_id = we.id AND viewer.user_id = ?
		JOIN media_items media ON media.id = we.media_id
		LEFT JOIN episodes episode ON episode.id = we.episode_id
		LEFT JOIN seasons season ON season.id = episode.season_id
		JOIN watch_event_participants participant ON participant.event_id = we.id
		JOIN users participant_user ON participant_user.id = participant.user_id
		WHERE we.watched_at >= ? AND we.watched_at < ?`
	arguments := []any{userID, formatEventTime(start), formatEventTime(end)}
	switch filter {
	case CalendarFilterCompleted:
		query += " AND round.status = ?"
		arguments = append(arguments, StatusCompleted)
	case CalendarFilterInProgress:
		query += " AND round.status = ?"
		arguments = append(arguments, StatusWatching)
	}
	query += " ORDER BY we.watched_at, we.id, participant_user.username"

	rows, err := repository.db.Reader().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]CalendarEvent, 0)
	for rows.Next() {
		var event CalendarEvent
		var episodeID, watchedAt, viewingMethod sql.NullString
		var seasonNumber, episodeNumber, absoluteNumber sql.NullInt64
		var participant string
		if err := rows.Scan(
			&event.ID, &event.MediaID, &event.MediaType, &event.Title,
			&episodeID, &seasonNumber, &episodeNumber, &absoluteNumber,
			&watchedAt, &viewingMethod, &event.Status, &participant,
		); err != nil {
			return nil, err
		}
		if !watchedAt.Valid {
			return nil, ErrInvalidWatchEvent
		}
		event.WatchedAt, err = time.Parse(eventTimeLayout, watchedAt.String)
		if err != nil {
			return nil, err
		}
		setCalendarNullableFields(&event, episodeID, seasonNumber, episodeNumber, absoluteNumber, viewingMethod)
		if len(events) > 0 && events[len(events)-1].ID == event.ID {
			events[len(events)-1].Participants = append(events[len(events)-1].Participants, participant)
			continue
		}
		event.Participants = []string{participant}
		events = append(events, event)
	}
	return events, rows.Err()
}

func setCalendarNullableFields(
	event *CalendarEvent,
	episodeID sql.NullString,
	seasonNumber, episodeNumber, absoluteNumber sql.NullInt64,
	viewingMethod sql.NullString,
) {
	if episodeID.Valid {
		value := episodeID.String
		event.EpisodeID = &value
	}
	if seasonNumber.Valid {
		value := int(seasonNumber.Int64)
		event.SeasonNumber = &value
	}
	if episodeNumber.Valid {
		value := int(episodeNumber.Int64)
		event.EpisodeNumber = &value
	}
	if absoluteNumber.Valid {
		value := int(absoluteNumber.Int64)
		event.AbsoluteNumber = &value
	}
	if viewingMethod.Valid {
		value := viewingMethod.String
		event.ViewingMethod = &value
	}
}
