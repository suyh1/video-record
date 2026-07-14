package records

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

func (repository *SQLiteRepository) ValidateRoundScope(ctx context.Context, scope RoundScope) error {
	if scope.UserID == "" || scope.MediaID == "" || scope.SeasonNumber != nil && *scope.SeasonNumber < 1 {
		return ErrInvalidRoundScope
	}
	var mediaType string
	err := repository.db.Reader().QueryRowContext(ctx,
		"SELECT media_type FROM media_items WHERE id = ?", scope.MediaID,
	).Scan(&mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidRoundScope
	}
	if err != nil {
		return err
	}
	if mediaType == "movie" && scope.SeasonNumber != nil || mediaType == "tv" && scope.SeasonNumber == nil {
		return ErrInvalidRoundScope
	}
	return nil
}

func (repository *SQLiteRepository) FindCurrentRound(ctx context.Context, scope RoundScope) (WatchRound, bool, error) {
	return repository.findRound(ctx, scope, "AND archived_at IS NULL ORDER BY round_number DESC LIMIT 1")
}

func (repository *SQLiteRepository) FindLatestRound(ctx context.Context, scope RoundScope) (WatchRound, bool, error) {
	return repository.findRound(ctx, scope, "ORDER BY round_number DESC LIMIT 1")
}

func (repository *SQLiteRepository) findRound(
	ctx context.Context,
	scope RoundScope,
	suffix string,
) (WatchRound, bool, error) {
	query := `
		SELECT id, user_id, media_id, season_number, round_number, status,
		       rating, note, viewing_method, started_at, completed_at, archived_at,
		       version, status_source, rating_source, note_source
		FROM watch_rounds
		WHERE user_id = ? AND media_id = ? AND season_number IS ? ` + suffix
	row := repository.db.Reader().QueryRowContext(ctx, query, scope.UserID, scope.MediaID, nullableInt(scope.SeasonNumber))
	round, err := scanRound(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WatchRound{}, false, nil
	}
	return round, err == nil, err
}

func (repository *SQLiteRepository) InsertRound(ctx context.Context, round WatchRound) error {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertRound(ctx, tx, round); err != nil {
		return err
	}
	if err := projectMediaProfile(ctx, tx, round.UserID, round.MediaID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertRound(ctx context.Context, executor sqlExecutor, round WatchRound) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status,
			rating, note, viewing_method, started_at, completed_at, archived_at,
			version, status_source, rating_source, note_source, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		          strftime('%s', 'now') * 1000, strftime('%s', 'now') * 1000)
	`, round.ID, round.UserID, round.MediaID, nullableInt(round.SeasonNumber), round.RoundNumber,
		round.Status, nullableInt(round.Rating), nullableText(round.Note), nullableText(round.ViewingMethod),
		nullableEventTime(round.StartedAt), nullableEventTime(round.CompletedAt), nullableEventTime(round.ArchivedAt),
		round.Version, round.StatusSource, round.RatingSource, round.NoteSource)
	return err
}

func (repository *SQLiteRepository) UpdateRound(ctx context.Context, round WatchRound, expectedVersion int) (bool, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE watch_rounds SET
			status = ?, rating = ?, note = ?, viewing_method = ?,
			started_at = ?, completed_at = ?, version = ?,
			status_source = ?, rating_source = ?, note_source = ?,
			updated_at = strftime('%s', 'now') * 1000
		WHERE id = ? AND user_id = ? AND version = ? AND archived_at IS NULL
	`, round.Status, nullableInt(round.Rating), nullableText(round.Note), nullableText(round.ViewingMethod),
		nullableEventTime(round.StartedAt), nullableEventTime(round.CompletedAt), round.Version,
		round.StatusSource, round.RatingSource, round.NoteSource,
		round.ID, round.UserID, expectedVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return false, err
	}
	if err := projectMediaProfile(ctx, tx, round.UserID, round.MediaID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (repository *SQLiteRepository) ArchiveCurrentRound(
	ctx context.Context,
	scope RoundScope,
	expectedVersion int,
	archivedAt time.Time,
) (RewatchResult, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return RewatchResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, exists, err := findCurrentRoundTx(ctx, tx, scope)
	if err != nil {
		return RewatchResult{}, err
	}
	if !exists || current.Status != StatusCompleted {
		return RewatchResult{}, ErrRoundNotCompleted
	}
	if current.Version != expectedVersion {
		return RewatchResult{}, ErrVersionConflict
	}
	archivedAt = archivedAt.UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE watch_rounds
		SET archived_at = ?, updated_at = strftime('%s', 'now') * 1000
		WHERE id = ? AND user_id = ? AND version = ? AND archived_at IS NULL
	`, formatEventTime(archivedAt), current.ID, current.UserID, expectedVersion)
	if err != nil {
		return RewatchResult{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return RewatchResult{}, err
	}
	if rows != 1 {
		return RewatchResult{}, ErrVersionConflict
	}
	archived := current
	archived.ArchivedAt = timePointerCopy(archivedAt)
	next := WatchRound{
		ID: uuid.NewString(), UserID: scope.UserID, MediaID: scope.MediaID,
		SeasonNumber: cloneIntPointer(scope.SeasonNumber), RoundNumber: current.RoundNumber + 1,
		Status: StatusWatching, StartedAt: timePointerCopy(archivedAt), Version: 1,
		StatusSource: SourceManual, RatingSource: SourceManual, NoteSource: SourceManual,
	}
	if err := insertRound(ctx, tx, next); err != nil {
		return RewatchResult{}, err
	}
	if err := projectMediaProfile(ctx, tx, scope.UserID, scope.MediaID); err != nil {
		return RewatchResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RewatchResult{}, err
	}
	return RewatchResult{Archived: archived, Current: next}, nil
}

func (repository *SQLiteRepository) ArchivedRounds(
	ctx context.Context,
	scope RoundScope,
) ([]RoundSummary, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT id, user_id, media_id, season_number, round_number, status,
		       rating, note, viewing_method, started_at, completed_at, archived_at,
		       version, status_source, rating_source, note_source
		FROM watch_rounds
		WHERE user_id = ? AND media_id = ? AND season_number IS ? AND archived_at IS NOT NULL
		ORDER BY round_number DESC, archived_at DESC, id DESC
	`, scope.UserID, scope.MediaID, nullableInt(scope.SeasonNumber))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	history := make([]RoundSummary, 0)
	for rows.Next() {
		round, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		history = append(history, RoundSummary{
			ID: round.ID, MediaID: round.MediaID, SeasonNumber: round.SeasonNumber,
			RoundNumber: round.RoundNumber, CompletedAt: round.CompletedAt, Rating: round.Rating,
		})
	}
	return history, rows.Err()
}

func (repository *SQLiteRepository) FindArchivedRoundDetail(
	ctx context.Context,
	scope RoundScope,
	roundID string,
) (RoundDetail, bool, error) {
	round, err := scanRound(repository.db.Reader().QueryRowContext(ctx, `
		SELECT id, user_id, media_id, season_number, round_number, status,
		       rating, note, viewing_method, started_at, completed_at, archived_at,
		       version, status_source, rating_source, note_source
		FROM watch_rounds
		WHERE id = ? AND user_id = ? AND media_id = ? AND season_number IS ?
		  AND archived_at IS NOT NULL
	`, roundID, scope.UserID, scope.MediaID, nullableInt(scope.SeasonNumber)))
	if errors.Is(err, sql.ErrNoRows) {
		return RoundDetail{}, false, nil
	}
	if err != nil {
		return RoundDetail{}, false, err
	}
	detail := RoundDetail{Round: round, Episodes: make([]Episode, 0)}
	if scope.SeasonNumber == nil {
		return detail, true, nil
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
	`, round.ID, scope.MediaID, *scope.SeasonNumber)
	if err != nil {
		return RoundDetail{}, false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var episode Episode
		var watchedAt sql.NullString
		if err := rows.Scan(
			&episode.ID, &episode.SourceID, &episode.SeasonID, &episode.SeasonNumber,
			&episode.EpisodeNumber, &episode.AbsoluteNumber, &episode.Name, &watchedAt,
		); err != nil {
			return RoundDetail{}, false, err
		}
		if watchedAt.Valid {
			value, err := time.Parse(eventTimeLayout, watchedAt.String)
			if err != nil {
				return RoundDetail{}, false, err
			}
			episode.Watched = true
			episode.WatchedAt = &value
		}
		detail.Episodes = append(detail.Episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return RoundDetail{}, false, err
	}
	return detail, true, nil
}

type roundScanner interface {
	Scan(...any) error
}

func scanRound(scanner roundScanner) (WatchRound, error) {
	var round WatchRound
	var seasonNumber, rating sql.NullInt64
	var note, viewingMethod sql.NullString
	var startedAt, completedAt, archivedAt sql.NullString
	err := scanner.Scan(
		&round.ID, &round.UserID, &round.MediaID, &seasonNumber, &round.RoundNumber, &round.Status,
		&rating, &note, &viewingMethod, &startedAt, &completedAt, &archivedAt,
		&round.Version, &round.StatusSource, &round.RatingSource, &round.NoteSource,
	)
	if err != nil {
		return WatchRound{}, err
	}
	if seasonNumber.Valid {
		value := int(seasonNumber.Int64)
		round.SeasonNumber = &value
	}
	if rating.Valid {
		value := int(rating.Int64)
		round.Rating = &value
	}
	if note.Valid {
		round.Note = stringPointer(note.String)
	}
	if viewingMethod.Valid {
		round.ViewingMethod = stringPointer(viewingMethod.String)
	}
	var parseErr error
	round.StartedAt, parseErr = parseRoundTime(startedAt)
	if parseErr != nil {
		return WatchRound{}, parseErr
	}
	round.CompletedAt, parseErr = parseRoundTime(completedAt)
	if parseErr != nil {
		return WatchRound{}, parseErr
	}
	round.ArchivedAt, parseErr = parseRoundTime(archivedAt)
	return round, parseErr
}

func parseRoundTime(value sql.NullString) (*time.Time, error) {
	return parseNullableEventTime(value)
}
