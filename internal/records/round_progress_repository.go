package records

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

func (repository *SQLiteRepository) SeasonEpisodes(
	ctx context.Context,
	userID, mediaID string,
	seasonNumber int,
) (SeriesProgress, error) {
	round, exists, err := repository.FindCurrentRound(ctx, RoundScope{
		UserID: userID, MediaID: mediaID, SeasonNumber: &seasonNumber,
	})
	if err != nil {
		return SeriesProgress{}, err
	}
	roundID := ""
	if exists {
		roundID = round.ID
	}
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT episode.id, COALESCE(episode.source_id, ''), season.id, season.season_number,
		       episode.episode_number,
		       COALESCE(episode.absolute_number, ROW_NUMBER() OVER (ORDER BY episode.episode_number)),
		       episode.name, progress.watched_at
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		LEFT JOIN round_episode_progress progress
		  ON progress.episode_id = episode.id AND progress.round_id = ?
		WHERE season.media_id = ? AND season.season_number = ?
		ORDER BY episode.episode_number
	`, roundID, mediaID, seasonNumber)
	if err != nil {
		return SeriesProgress{}, err
	}
	defer func() { _ = rows.Close() }()

	progress := SeriesProgress{
		RoundID: roundID, MediaID: mediaID, SeasonNumber: seasonNumber,
		Status: StatusNone, Episodes: make([]Episode, 0),
	}
	if exists {
		progress.Status = round.Status
		progress.Version = round.Version
	}
	for rows.Next() {
		var episode Episode
		var watchedAt sql.NullString
		if err := rows.Scan(
			&episode.ID, &episode.SourceID, &episode.SeasonID, &episode.SeasonNumber,
			&episode.EpisodeNumber, &episode.AbsoluteNumber, &episode.Name, &watchedAt,
		); err != nil {
			return SeriesProgress{}, err
		}
		if watchedAt.Valid {
			value, err := time.Parse(eventTimeLayout, watchedAt.String)
			if err != nil {
				return SeriesProgress{}, err
			}
			episode.Watched = true
			episode.WatchedAt = &value
			progress.WatchedEpisodes++
			if progress.LastWatched == nil || value.After(*progress.LastWatched.WatchedAt) {
				copy := episode
				progress.LastWatched = &copy
			}
		} else if progress.NextEpisode == nil {
			copy := episode
			progress.NextEpisode = &copy
		}
		progress.Episodes = append(progress.Episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return SeriesProgress{}, err
	}
	progress.TotalEpisodes = len(progress.Episodes)
	return progress, nil
}

func (repository *SQLiteRepository) ApplySeasonEpisodeProgress(
	ctx context.Context,
	input EpisodeProgressInput,
	episodeIDs []string,
) (bool, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	current, exists, err := findCurrentRoundTx(ctx, tx, RoundScope{
		UserID: input.UserID, MediaID: input.MediaID, SeasonNumber: &input.SeasonNumber,
	})
	if err != nil {
		return false, err
	}
	if !exists {
		current = emptyRound(RoundScope{
			UserID: input.UserID, MediaID: input.MediaID, SeasonNumber: &input.SeasonNumber,
		})
	}
	if current.Version != input.ExpectedVersion {
		return false, ErrVersionConflict
	}
	if len(input.EpisodeRefs) > 0 {
		episodeIDs, err = ensureEpisodeIdentities(ctx, tx, input.MediaID, input.EpisodeRefs)
		if err != nil {
			return false, err
		}
	}
	if err := validateSeasonEpisodeIDs(ctx, tx, input.MediaID, input.SeasonNumber, episodeIDs); err != nil {
		return false, err
	}

	if !exists && input.Action != EpisodeProgressUndo {
		current.ID = uuid.NewString()
		current.RoundNumber, err = nextRoundNumber(ctx, tx, input.UserID, input.MediaID, input.SeasonNumber)
		if err != nil {
			return false, err
		}
		current.Version = 1
		current.StatusSource = input.Source
		current.RatingSource = input.Source
		current.NoteSource = input.Source
		if err := insertRound(ctx, tx, current); err != nil {
			return false, err
		}
		exists = true
	}

	changed := false
	for _, episodeID := range episodeIDs {
		var eventID, watchedAt string
		err := sql.ErrNoRows
		if exists {
			err = tx.QueryRowContext(ctx, `
				SELECT watch_event_id, watched_at
				FROM round_episode_progress
				WHERE round_id = ? AND episode_id = ?
			`, current.ID, episodeID).Scan(&eventID, &watchedAt)
		}
		switch {
		case input.Action == EpisodeProgressUndo && err == nil:
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM round_episode_progress WHERE round_id = ? AND episode_id = ?
			`, current.ID, episodeID); err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM watch_events WHERE id = ? AND round_id = ? AND created_by_user_id = ?
			`, eventID, current.ID, input.UserID); err != nil {
				return false, err
			}
			changed = true
		case input.Action == EpisodeProgressUndo && errors.Is(err, sql.ErrNoRows):
			continue
		case err == nil && input.Action == EpisodeProgressSetTime:
			formatted := formatEventTime(input.WatchedAt)
			if watchedAt == formatted {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE round_episode_progress
				SET watched_at = ?, updated_at = strftime('%s', 'now') * 1000
				WHERE round_id = ? AND episode_id = ?
			`, formatted, current.ID, episodeID); err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE watch_events SET watched_at = ? WHERE id = ? AND round_id = ?
			`, formatted, eventID, current.ID); err != nil {
				return false, err
			}
			changed = true
		case err == nil:
			continue
		case errors.Is(err, sql.ErrNoRows):
			event, err := newWatchEvent(CreateWatchEventInput{
				RoundID: current.ID, UserID: input.UserID, MediaID: input.MediaID,
				EpisodeID: episodeID, WatchedAt: input.WatchedAt,
				Source: input.Source, Completion: 100,
			})
			if err != nil {
				return false, err
			}
			if err := insertWatchEvent(ctx, tx, event); err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO round_episode_progress (
					round_id, episode_id, watched_at, source, watch_event_id, updated_at
				) VALUES (?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
			`, current.ID, episodeID, formatEventTime(input.WatchedAt), input.Source, event.ID); err != nil {
				return false, err
			}
			changed = true
		default:
			return false, err
		}
	}
	if !changed {
		return false, tx.Commit()
	}

	var watchedCount, totalCount int
	var firstWatchedAt, lastWatchedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(progress.episode_id), COUNT(episode.id),
		       MIN(progress.watched_at), MAX(progress.watched_at)
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		LEFT JOIN round_episode_progress progress
		  ON progress.episode_id = episode.id AND progress.round_id = ?
		WHERE season.media_id = ? AND season.season_number = ?
	`, current.ID, input.MediaID, input.SeasonNumber).Scan(
		&watchedCount, &totalCount, &firstWatchedAt, &lastWatchedAt,
	); err != nil {
		return false, err
	}
	if input.TotalEpisodes > totalCount {
		totalCount = input.TotalEpisodes
	}
	projectedStatus := projectedSeriesStatus(watchedCount, totalCount)
	if CanOverwrite(input.Source, current.StatusSource) {
		current.Status = projectedStatus
		current.StatusSource = input.Source
	}
	current.StartedAt, err = parseNullableEventTime(firstWatchedAt)
	if err != nil {
		return false, err
	}
	current.CompletedAt = nil
	if current.Status == StatusCompleted {
		current.CompletedAt, err = parseNullableEventTime(lastWatchedAt)
		if err != nil {
			return false, err
		}
	}

	storedVersion := current.Version
	if storedVersion == 0 {
		return false, ErrVersionConflict
	}
	if input.ExpectedVersion > 0 {
		current.Version = input.ExpectedVersion + 1
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE watch_rounds SET
			status = ?, started_at = ?, completed_at = ?, version = ?, status_source = ?,
			updated_at = strftime('%s', 'now') * 1000
		WHERE id = ? AND user_id = ? AND version = ? AND archived_at IS NULL
	`, current.Status, nullableEventTime(current.StartedAt), nullableEventTime(current.CompletedAt),
		current.Version, current.StatusSource, current.ID, current.UserID, storedVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows != 1 {
		return false, ErrVersionConflict
	}
	if err := projectMediaProfile(ctx, tx, input.UserID, input.MediaID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func findCurrentRoundTx(ctx context.Context, tx *sql.Tx, scope RoundScope) (WatchRound, bool, error) {
	round, err := scanRound(tx.QueryRowContext(ctx, `
		SELECT id, user_id, media_id, season_number, round_number, status,
		       rating, note, viewing_method, started_at, completed_at, archived_at,
		       version, status_source, rating_source, note_source
		FROM watch_rounds
		WHERE user_id = ? AND media_id = ? AND season_number IS ? AND archived_at IS NULL
		ORDER BY round_number DESC LIMIT 1
	`, scope.UserID, scope.MediaID, nullableInt(scope.SeasonNumber)))
	if errors.Is(err, sql.ErrNoRows) {
		return WatchRound{}, false, nil
	}
	return round, err == nil, err
}

func validateSeasonEpisodeIDs(
	ctx context.Context,
	tx *sql.Tx,
	mediaID string,
	seasonNumber int,
	episodeIDs []string,
) error {
	if len(episodeIDs) == 0 {
		return ErrEpisodeNotFound
	}
	for _, episodeID := range episodeIDs {
		var count int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM episodes episode
			JOIN seasons season ON season.id = episode.season_id
			WHERE episode.id = ? AND season.media_id = ? AND season.season_number = ?
		`, episodeID, mediaID, seasonNumber).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return ErrEpisodeNotFound
		}
	}
	return nil
}

func nextRoundNumber(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID string,
	seasonNumber int,
) (int, error) {
	var number int
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(round_number), 0) + 1
		FROM watch_rounds
		WHERE user_id = ? AND media_id = ? AND season_number = ?
	`, userID, mediaID, seasonNumber).Scan(&number)
	return number, err
}
