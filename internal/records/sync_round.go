package records

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type SyncedWatchInput struct {
	UserID          string
	MediaID         string
	EpisodeID       string
	WatchedAt       time.Time
	ViewingMethod   string
	ExternalEventID string
	Completion      int
	Now             time.Time
}

type SyncedWatchResult struct {
	RoundID string
	EventID string
}

func ApplySyncedWatch(
	ctx context.Context,
	tx *sql.Tx,
	input SyncedWatchInput,
) (SyncedWatchResult, error) {
	if input.UserID == "" || input.MediaID == "" || input.ExternalEventID == "" ||
		input.WatchedAt.IsZero() || input.Completion < 0 || input.Completion > 100 {
		return SyncedWatchResult{}, ErrInvalidWatchEvent
	}
	input.WatchedAt = input.WatchedAt.UTC()
	input.Now = input.Now.UTC()
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}
	scope, err := syncedRoundScope(ctx, tx, input.MediaID, input.EpisodeID)
	if err != nil {
		return SyncedWatchResult{}, err
	}
	scope.UserID = input.UserID
	current, exists, err := findCurrentRoundTx(ctx, tx, scope)
	if err != nil {
		return SyncedWatchResult{}, err
	}
	if exists && !CanOverwrite(SourceConfirmedSync, current.StatusSource) {
		return SyncedWatchResult{}, ErrVersionConflict
	}
	if exists && current.Status == StatusCompleted {
		if _, err := tx.ExecContext(ctx, `
			UPDATE watch_rounds
			SET archived_at = ?, updated_at = strftime('%s', 'now') * 1000
			WHERE id = ? AND version = ? AND archived_at IS NULL
		`, formatEventTime(input.Now), current.ID, current.Version); err != nil {
			return SyncedWatchResult{}, err
		}
		exists = false
	}
	if !exists {
		roundNumber, err := nextSyncedRoundNumber(ctx, tx, scope)
		if err != nil {
			return SyncedWatchResult{}, err
		}
		current = WatchRound{
			ID: uuid.NewString(), UserID: input.UserID, MediaID: input.MediaID,
			SeasonNumber: cloneIntPointer(scope.SeasonNumber), RoundNumber: roundNumber,
			Status: StatusNone, Version: 1,
			StatusSource: SourceConfirmedSync, RatingSource: SourceConfirmedSync, NoteSource: SourceConfirmedSync,
		}
		if err := insertRound(ctx, tx, current); err != nil {
			return SyncedWatchResult{}, err
		}
	}
	event, err := newWatchEvent(CreateWatchEventInput{
		RoundID: current.ID, UserID: input.UserID, MediaID: input.MediaID,
		EpisodeID: input.EpisodeID, WatchedAt: input.WatchedAt,
		ViewingMethod: input.ViewingMethod, Source: SourceConfirmedSync,
		ExternalEventID: input.ExternalEventID, Completion: input.Completion,
	})
	if err != nil {
		return SyncedWatchResult{}, err
	}
	if err := insertWatchEvent(ctx, tx, event); err != nil {
		return SyncedWatchResult{}, err
	}
	if input.EpisodeID != "" && input.Completion >= 90 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO round_episode_progress (
				round_id, episode_id, watched_at, source, watch_event_id, updated_at
			) VALUES (?, ?, ?, 'confirmed_sync', ?, strftime('%s', 'now') * 1000)
			ON CONFLICT(round_id, episode_id) DO NOTHING
		`, current.ID, input.EpisodeID, formatEventTime(input.WatchedAt), event.ID); err != nil {
			return SyncedWatchResult{}, err
		}
	}

	status, startedAt, completedAt, err := syncedRoundProjection(ctx, tx, current, input)
	if err != nil {
		return SyncedWatchResult{}, err
	}
	storedVersion := current.Version
	if exists {
		current.Version++
	}
	current.Status = status
	current.StatusSource = SourceConfirmedSync
	current.StartedAt = startedAt
	current.CompletedAt = completedAt
	result, err := tx.ExecContext(ctx, `
		UPDATE watch_rounds SET
			status = ?, started_at = ?, completed_at = ?, version = ?, status_source = ?,
			updated_at = strftime('%s', 'now') * 1000
		WHERE id = ? AND version = ? AND archived_at IS NULL
	`, current.Status, nullableEventTime(current.StartedAt), nullableEventTime(current.CompletedAt),
		current.Version, current.StatusSource, current.ID, storedVersion)
	if err != nil {
		return SyncedWatchResult{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return SyncedWatchResult{}, err
	}
	if rows != 1 {
		return SyncedWatchResult{}, ErrVersionConflict
	}
	if err := projectMediaProfile(ctx, tx, input.UserID, input.MediaID); err != nil {
		return SyncedWatchResult{}, err
	}
	return SyncedWatchResult{RoundID: current.ID, EventID: event.ID}, nil
}

func syncedRoundScope(
	ctx context.Context,
	tx *sql.Tx,
	mediaID, episodeID string,
) (RoundScope, error) {
	scope := RoundScope{MediaID: mediaID}
	var mediaType string
	if err := tx.QueryRowContext(ctx, "SELECT media_type FROM media_items WHERE id = ?", mediaID).Scan(&mediaType); err != nil {
		return RoundScope{}, ErrInvalidRoundScope
	}
	if episodeID == "" {
		if mediaType != "movie" {
			return RoundScope{}, ErrInvalidRoundScope
		}
		return scope, nil
	}
	if mediaType != "tv" {
		return RoundScope{}, ErrInvalidRoundScope
	}
	var seasonNumber int
	err := tx.QueryRowContext(ctx, `
		SELECT season.season_number
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		WHERE episode.id = ? AND season.media_id = ?
	`, episodeID, mediaID).Scan(&seasonNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return RoundScope{}, ErrEpisodeNotFound
	}
	if err != nil {
		return RoundScope{}, err
	}
	scope.SeasonNumber = &seasonNumber
	return scope, nil
}

func nextSyncedRoundNumber(ctx context.Context, tx *sql.Tx, scope RoundScope) (int, error) {
	var number int
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(round_number), 0) + 1
		FROM watch_rounds
		WHERE user_id = ? AND media_id = ? AND season_number IS ?
	`, scope.UserID, scope.MediaID, nullableInt(scope.SeasonNumber)).Scan(&number)
	return number, err
}

func syncedRoundProjection(
	ctx context.Context,
	tx *sql.Tx,
	round WatchRound,
	input SyncedWatchInput,
) (Status, *time.Time, *time.Time, error) {
	var firstEvent sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT MIN(watched_at) FROM watch_events WHERE round_id = ?
	`, round.ID).Scan(&firstEvent); err != nil {
		return "", nil, nil, err
	}
	startedAt, err := parseNullableEventTime(firstEvent)
	if err != nil {
		return "", nil, nil, err
	}
	if input.EpisodeID == "" {
		if input.Completion < 90 {
			return StatusWatching, startedAt, nil, nil
		}
		var lastEvent sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT MAX(watched_at) FROM watch_events WHERE round_id = ?
		`, round.ID).Scan(&lastEvent); err != nil {
			return "", nil, nil, err
		}
		completedAt, err := parseNullableEventTime(lastEvent)
		return StatusCompleted, startedAt, completedAt, err
	}
	var watched, total int
	var lastProgress sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(progress.episode_id), COUNT(episode.id), MAX(progress.watched_at)
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		LEFT JOIN round_episode_progress progress
		  ON progress.episode_id = episode.id AND progress.round_id = ?
		WHERE season.media_id = ? AND season.season_number = ?
	`, round.ID, input.MediaID, *round.SeasonNumber).Scan(&watched, &total, &lastProgress); err != nil {
		return "", nil, nil, err
	}
	status := projectedSeriesStatus(watched, total)
	if input.Completion < 90 && status == StatusNone {
		status = StatusWatching
	}
	if status != StatusCompleted {
		return status, startedAt, nil, nil
	}
	completedAt, err := parseNullableEventTime(lastProgress)
	return status, startedAt, completedAt, err
}
