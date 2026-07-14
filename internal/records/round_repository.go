package records

import (
	"context"
	"database/sql"
	"errors"
	"time"
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
