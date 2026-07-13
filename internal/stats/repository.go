package stats

import (
	"context"
	"time"

	"video-record/internal/storage"
)

const eventTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

type SQLiteRepository struct {
	db *storage.DB
}

func NewRepository(db *storage.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (repository *SQLiteRepository) EventFacts(ctx context.Context, userID string) ([]EventFact, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT event.media_id, event.watched_at,
		       COALESCE(episode.runtime, media.runtime_minutes, 0),
		       COALESCE(event.viewing_method, '')
		FROM watch_events event
		JOIN watch_event_participants participant
		  ON participant.event_id = event.id AND participant.user_id = ?
		JOIN media_items media ON media.id = event.media_id
		LEFT JOIN episodes episode ON episode.id = event.episode_id
		ORDER BY event.watched_at, event.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	facts := make([]EventFact, 0)
	for rows.Next() {
		var fact EventFact
		var watchedAt string
		if err := rows.Scan(&fact.MediaID, &watchedAt, &fact.RuntimeMinutes, &fact.ViewingMethod); err != nil {
			return nil, err
		}
		fact.WatchedAt, err = time.Parse(eventTimeLayout, watchedAt)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

func (repository *SQLiteRepository) GenreCounts(ctx context.Context, userID string) ([]Point, error) {
	return repository.dimensionCounts(ctx, `
		SELECT genre.name, COUNT(*)
		FROM watch_events event
		JOIN watch_event_participants participant
		  ON participant.event_id = event.id AND participant.user_id = ?
		JOIN media_genres media_genre ON media_genre.media_id = event.media_id
		JOIN genres genre ON genre.source = media_genre.source AND genre.source_id = media_genre.source_id
		GROUP BY genre.source, genre.source_id, genre.name
		ORDER BY COUNT(*) DESC, genre.name
	`, userID)
}

func (repository *SQLiteRepository) Ratings(ctx context.Context, userID string) ([]int, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT rating FROM user_media_states
		WHERE user_id = ? AND rating IS NOT NULL
		ORDER BY rating
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	values := make([]int, 0)
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (repository *SQLiteRepository) TagCounts(ctx context.Context, userID string) ([]Point, error) {
	return repository.dimensionCounts(ctx, `
		SELECT tag.name, COUNT(*)
		FROM user_media_tags media_tag
		JOIN tags tag ON tag.id = media_tag.tag_id
		WHERE media_tag.user_id = ?
		GROUP BY tag.id, tag.name
		ORDER BY COUNT(*) DESC, tag.name
	`, userID)
}

func (repository *SQLiteRepository) dimensionCounts(ctx context.Context, query string, userID string) ([]Point, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	points := make([]Point, 0)
	for rows.Next() {
		var point Point
		if err := rows.Scan(&point.Label, &point.Value); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}
