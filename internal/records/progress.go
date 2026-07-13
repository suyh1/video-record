package records

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrEpisodeNotFound        = errors.New("episode not found")
	ErrInvalidEpisodeProgress = errors.New("invalid episode progress")
)

type EpisodeProgressAction string

const (
	EpisodeProgressSingle EpisodeProgressAction = "single"
	EpisodeProgressRange  EpisodeProgressAction = "range"
	EpisodeProgressSeason EpisodeProgressAction = "season"
	EpisodeProgressNext   EpisodeProgressAction = "next"
	EpisodeProgressUndo   EpisodeProgressAction = "undo"
)

type EpisodeProgressInput struct {
	UserID           string
	MediaID          string
	Action           EpisodeProgressAction
	EpisodeID        string
	ThroughEpisodeID string
	SeasonID         string
	WatchedAt        time.Time
	Source           Source
	ExpectedVersion  int
}

type Episode struct {
	ID             string
	SeasonID       string
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
	Name           string
	Watched        bool
	WatchedAt      *time.Time
}

type SeriesProgress struct {
	MediaID         string
	Status          Status
	Version         int
	WatchedEpisodes int
	TotalEpisodes   int
	LastWatched     *Episode
	NextEpisode     *Episode
	Episodes        []Episode
}

func (service *Service) EpisodeProgress(ctx context.Context, userID, mediaID string) (SeriesProgress, error) {
	if userID == "" || mediaID == "" {
		return SeriesProgress{}, ErrInvalidEpisodeProgress
	}
	return service.repository.Episodes(ctx, userID, mediaID)
}

func (service *Service) UpdateEpisodeProgress(ctx context.Context, input EpisodeProgressInput) (SeriesProgress, error) {
	if input.UserID == "" || input.MediaID == "" || sourcePriority(input.Source) == 0 {
		return SeriesProgress{}, ErrInvalidEpisodeProgress
	}
	current, err := service.repository.Episodes(ctx, input.UserID, input.MediaID)
	if err != nil {
		return SeriesProgress{}, err
	}
	if current.Version != input.ExpectedVersion {
		return current, ErrVersionConflict
	}
	targets, watched, err := selectProgressTargets(current.Episodes, input)
	if err != nil {
		return current, err
	}
	if input.WatchedAt.IsZero() {
		input.WatchedAt = time.Now().UTC()
	}
	_, err = service.repository.ApplyEpisodeProgress(ctx, input, targets, watched)
	if err != nil {
		return current, err
	}
	return service.repository.Episodes(ctx, input.UserID, input.MediaID)
}

func selectProgressTargets(episodes []Episode, input EpisodeProgressInput) ([]string, bool, error) {
	indexOf := func(id string) int {
		for index := range episodes {
			if episodes[index].ID == id {
				return index
			}
		}
		return -1
	}
	switch input.Action {
	case EpisodeProgressSingle, EpisodeProgressUndo:
		if indexOf(input.EpisodeID) < 0 {
			return nil, false, ErrEpisodeNotFound
		}
		return []string{input.EpisodeID}, input.Action != EpisodeProgressUndo, nil
	case EpisodeProgressRange:
		start, end := indexOf(input.EpisodeID), indexOf(input.ThroughEpisodeID)
		if start < 0 || end < start {
			return nil, false, ErrInvalidEpisodeProgress
		}
		targets := make([]string, 0, end-start+1)
		for _, episode := range episodes[start : end+1] {
			targets = append(targets, episode.ID)
		}
		return targets, true, nil
	case EpisodeProgressSeason:
		targets := make([]string, 0)
		for _, episode := range episodes {
			if episode.SeasonID == input.SeasonID {
				targets = append(targets, episode.ID)
			}
		}
		if len(targets) == 0 {
			return nil, false, ErrEpisodeNotFound
		}
		return targets, true, nil
	case EpisodeProgressNext:
		for _, episode := range episodes {
			if !episode.Watched {
				return []string{episode.ID}, true, nil
			}
		}
		return nil, false, ErrEpisodeNotFound
	default:
		return nil, false, ErrInvalidEpisodeProgress
	}
}

func (repository *SQLiteRepository) Episodes(ctx context.Context, userID, mediaID string) (SeriesProgress, error) {
	rows, err := repository.db.Reader().QueryContext(ctx, `
		SELECT episode.id, season.id, season.season_number, episode.episode_number,
		       ROW_NUMBER() OVER (ORDER BY season.season_number, episode.episode_number),
		       episode.name, progress.watched_at
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		LEFT JOIN episode_progress progress
		  ON progress.episode_id = episode.id AND progress.user_id = ?
		WHERE season.media_id = ?
		ORDER BY season.season_number, episode.episode_number
	`, userID, mediaID)
	if err != nil {
		return SeriesProgress{}, err
	}
	defer func() { _ = rows.Close() }()
	progress := SeriesProgress{MediaID: mediaID, Status: StatusNone, Episodes: make([]Episode, 0)}
	for rows.Next() {
		var episode Episode
		var watchedAt sql.NullString
		if err := rows.Scan(
			&episode.ID, &episode.SeasonID, &episode.SeasonNumber, &episode.EpisodeNumber,
			&episode.AbsoluteNumber, &episode.Name, &watchedAt,
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
	state, exists, err := repository.FindState(ctx, userID, mediaID)
	if err != nil {
		return SeriesProgress{}, err
	}
	if exists {
		progress.Status = state.Status
		progress.Version = state.Version
	}
	return progress, nil
}

func (repository *SQLiteRepository) ApplyEpisodeProgress(
	ctx context.Context,
	input EpisodeProgressInput,
	episodeIDs []string,
	watched bool,
) (bool, error) {
	tx, err := repository.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, exists, err := findProgressState(ctx, tx, input.UserID, input.MediaID)
	if err != nil {
		return false, err
	}
	if current.Version != input.ExpectedVersion {
		return false, ErrVersionConflict
	}

	changed := false
	for _, episodeID := range episodeIDs {
		var valid int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM episodes episode
			JOIN seasons season ON season.id = episode.season_id
			WHERE episode.id = ? AND season.media_id = ?
		`, episodeID, input.MediaID).Scan(&valid); err != nil {
			return false, err
		}
		if valid != 1 {
			return false, ErrEpisodeNotFound
		}
		var eventID string
		err := tx.QueryRowContext(ctx, `
			SELECT watch_event_id FROM episode_progress WHERE user_id = ? AND episode_id = ?
		`, input.UserID, episodeID).Scan(&eventID)
		switch {
		case watched && errors.Is(err, sql.ErrNoRows):
			event, err := newWatchEvent(CreateWatchEventInput{
				UserID: input.UserID, MediaID: input.MediaID, EpisodeID: episodeID,
				WatchedAt: input.WatchedAt, Source: input.Source, Completion: 100,
			})
			if err != nil {
				return false, err
			}
			if err := insertWatchEvent(ctx, tx, event); err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO episode_progress (
					user_id, media_id, episode_id, watched_at, source, watch_event_id, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, strftime('%s', 'now') * 1000)
			`, input.UserID, input.MediaID, episodeID, formatEventTime(input.WatchedAt), input.Source, event.ID); err != nil {
				return false, err
			}
			changed = true
		case watched && err == nil:
			continue
		case !watched && err == nil:
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM episode_progress WHERE user_id = ? AND episode_id = ?
			`, input.UserID, episodeID); err != nil {
				return false, err
			}
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM watch_events WHERE id = ? AND created_by_user_id = ?
			`, eventID, input.UserID); err != nil {
				return false, err
			}
			changed = true
		case !watched && errors.Is(err, sql.ErrNoRows):
			continue
		case err != nil:
			return false, err
		}
	}
	if !changed {
		return false, tx.Commit()
	}

	var watchedCount, totalCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			COUNT(progress.episode_id),
			COUNT(episode.id)
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		LEFT JOIN episode_progress progress
		  ON progress.episode_id = episode.id AND progress.user_id = ?
		WHERE season.media_id = ?
	`, input.UserID, input.MediaID).Scan(&watchedCount, &totalCount); err != nil {
		return false, err
	}
	next := current
	if !exists {
		next = State{
			UserID: input.UserID, MediaID: input.MediaID, Status: StatusNone,
			StatusSource: input.Source, RatingSource: input.Source, NoteSource: input.Source,
		}
	}
	if next.Status != StatusDropped && (!exists || CanOverwrite(input.Source, current.StatusSource)) {
		next.Status = projectedSeriesStatus(watchedCount, totalCount)
		next.StatusSource = input.Source
	}
	next.Version = current.Version + 1
	if exists {
		updated, err := updateState(ctx, tx, next, current.Version)
		if err != nil {
			return false, err
		}
		if !updated {
			return false, ErrVersionConflict
		}
	} else if err := insertState(ctx, tx, next); err != nil {
		return false, err
	}
	if err := recomputeWatchDates(ctx, tx, input.UserID, input.MediaID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func projectedSeriesStatus(watched, total int) Status {
	if total > 0 && watched == total {
		return StatusCompleted
	}
	if watched > 0 {
		return StatusWatching
	}
	return StatusNone
}

func reprojectSeriesStateAfterEventDeletion(ctx context.Context, tx *sql.Tx, userID, mediaID string) error {
	current, exists, err := findProgressState(ctx, tx, userID, mediaID)
	if err != nil || !exists {
		return err
	}
	var watchedCount, totalCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(progress.episode_id), COUNT(episode.id)
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		LEFT JOIN episode_progress progress
		  ON progress.episode_id = episode.id AND progress.user_id = ?
		WHERE season.media_id = ?
	`, userID, mediaID).Scan(&watchedCount, &totalCount); err != nil {
		return err
	}
	next := current
	if next.Status != StatusDropped {
		next.Status = projectedSeriesStatus(watchedCount, totalCount)
		next.StatusSource = SourceManual
	}
	next.Version++
	updated, err := updateState(ctx, tx, next, current.Version)
	if err != nil {
		return err
	}
	if !updated {
		return ErrVersionConflict
	}
	return nil
}

func findProgressState(ctx context.Context, tx *sql.Tx, userID, mediaID string) (State, bool, error) {
	var state State
	var rating sql.NullInt64
	var note, startedAt, completedAt sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT status, rating, note, version, status_source, rating_source, note_source,
		       started_at, completed_at
		FROM user_media_states WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(
		&state.Status, &rating, &note, &state.Version,
		&state.StatusSource, &state.RatingSource, &state.NoteSource,
		&startedAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return State{UserID: userID, MediaID: mediaID, Status: StatusNone}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	state.UserID, state.MediaID = userID, mediaID
	if rating.Valid {
		value := int(rating.Int64)
		state.Rating = &value
	}
	if note.Valid {
		value := note.String
		state.Note = &value
	}
	state.StartedAt, err = parseNullableEventTime(startedAt)
	if err != nil {
		return State{}, false, err
	}
	state.CompletedAt, err = parseNullableEventTime(completedAt)
	return state, true, err
}
