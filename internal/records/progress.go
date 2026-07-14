package records

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrEpisodeNotFound        = errors.New("episode not found")
	ErrInvalidEpisodeProgress = errors.New("invalid episode progress")
)

type EpisodeProgressAction string

const (
	EpisodeProgressSingle  EpisodeProgressAction = "single"
	EpisodeProgressRange   EpisodeProgressAction = "range"
	EpisodeProgressSeason  EpisodeProgressAction = "season"
	EpisodeProgressNext    EpisodeProgressAction = "next"
	EpisodeProgressUndo    EpisodeProgressAction = "undo"
	EpisodeProgressSetTime EpisodeProgressAction = "set_time"
)

type EpisodeProgressInput struct {
	UserID           string
	MediaID          string
	SeasonNumber     int
	Action           EpisodeProgressAction
	EpisodeID        string
	ThroughEpisodeID string
	SeasonID         string
	WatchedAt        time.Time
	Source           Source
	ExpectedVersion  int
	EpisodeRefs      []EpisodeReference
	TotalEpisodes    int
}

type EpisodeReference struct {
	SourceID       string
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
}

type Episode struct {
	ID             string
	SourceID       string
	SeasonID       string
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
	Name           string
	Watched        bool
	WatchedAt      *time.Time
}

type SeriesProgress struct {
	RoundID         string
	MediaID         string
	SeasonNumber    int
	Status          Status
	Version         int
	WatchedEpisodes int
	TotalEpisodes   int
	LastWatched     *Episode
	NextEpisode     *Episode
	Episodes        []Episode
}

func (service *Service) EpisodeProgress(ctx context.Context, userID, mediaID string, seasons ...int) (SeriesProgress, error) {
	if userID == "" || mediaID == "" || len(seasons) != 1 || seasons[0] < 1 {
		return SeriesProgress{}, ErrInvalidEpisodeProgress
	}
	seasonNumber := seasons[0]
	if err := service.repository.ValidateRoundScope(ctx, RoundScope{
		UserID: userID, MediaID: mediaID, SeasonNumber: &seasonNumber,
	}); err != nil {
		return SeriesProgress{}, ErrInvalidEpisodeProgress
	}
	return service.repository.SeasonEpisodes(ctx, userID, mediaID, seasonNumber)
}

func (service *Service) UpdateEpisodeProgress(ctx context.Context, input EpisodeProgressInput) (SeriesProgress, error) {
	if input.UserID == "" || input.MediaID == "" || input.SeasonNumber < 1 || sourcePriority(input.Source) == 0 {
		return SeriesProgress{}, ErrInvalidEpisodeProgress
	}
	if err := service.repository.ValidateRoundScope(ctx, RoundScope{
		UserID: input.UserID, MediaID: input.MediaID, SeasonNumber: &input.SeasonNumber,
	}); err != nil {
		return SeriesProgress{}, ErrInvalidEpisodeProgress
	}
	current, err := service.repository.SeasonEpisodes(ctx, input.UserID, input.MediaID, input.SeasonNumber)
	if err != nil {
		return SeriesProgress{}, err
	}
	if current.Version != input.ExpectedVersion {
		return current, ErrVersionConflict
	}
	if input.Action != EpisodeProgressUndo {
		if input.WatchedAt.IsZero() {
			input.WatchedAt = service.now().UTC()
		}
		if input.WatchedAt.After(service.now().UTC()) {
			return current, ErrInvalidWatchedAt
		}
	}
	var targets []string
	if len(input.EpisodeRefs) > 0 {
		if err := validateEpisodeReferences(input); err != nil {
			return current, err
		}
	} else {
		var selectErr error
		targets, _, selectErr = selectProgressTargets(current.Episodes, input)
		if selectErr != nil {
			return current, selectErr
		}
	}
	changed, err := service.repository.ApplySeasonEpisodeProgress(ctx, input, targets)
	if err != nil {
		return current, err
	}
	if !changed {
		return current, nil
	}
	progress, err := service.repository.SeasonEpisodes(ctx, input.UserID, input.MediaID, input.SeasonNumber)
	if err == nil && input.TotalEpisodes > progress.TotalEpisodes {
		progress.TotalEpisodes = input.TotalEpisodes
	}
	return progress, err
}

func validateEpisodeReferences(input EpisodeProgressInput) error {
	switch input.Action {
	case EpisodeProgressSingle, EpisodeProgressRange, EpisodeProgressSeason, EpisodeProgressNext, EpisodeProgressUndo, EpisodeProgressSetTime:
	default:
		return ErrInvalidEpisodeProgress
	}
	if input.TotalEpisodes < 1 {
		return ErrInvalidEpisodeProgress
	}
	if (input.Action == EpisodeProgressSingle || input.Action == EpisodeProgressNext || input.Action == EpisodeProgressUndo || input.Action == EpisodeProgressSetTime) && len(input.EpisodeRefs) != 1 {
		return ErrInvalidEpisodeProgress
	}
	seenSources := make(map[string]struct{}, len(input.EpisodeRefs))
	seenNumbers := make(map[[2]int]struct{}, len(input.EpisodeRefs))
	for _, episode := range input.EpisodeRefs {
		if episode.SourceID == "" || episode.SeasonNumber != input.SeasonNumber || episode.EpisodeNumber < 1 ||
			episode.AbsoluteNumber < 1 {
			return ErrInvalidEpisodeProgress
		}
		if _, exists := seenSources[episode.SourceID]; exists {
			return ErrInvalidEpisodeProgress
		}
		seenSources[episode.SourceID] = struct{}{}
		number := [2]int{episode.SeasonNumber, episode.EpisodeNumber}
		if _, exists := seenNumbers[number]; exists {
			return ErrInvalidEpisodeProgress
		}
		seenNumbers[number] = struct{}{}
	}
	return nil
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
	case EpisodeProgressSingle, EpisodeProgressUndo, EpisodeProgressSetTime:
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
			if input.SeasonID == "" || episode.SeasonID == input.SeasonID {
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

func ensureEpisodeIdentities(
	ctx context.Context,
	tx *sql.Tx,
	mediaID string,
	episodes []EpisodeReference,
) ([]string, error) {
	var validSeries int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM media_items WHERE id = ? AND media_type = 'tv'
	`, mediaID).Scan(&validSeries); err != nil {
		return nil, err
	}
	if validSeries != 1 {
		return nil, ErrInvalidEpisodeProgress
	}
	ids := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		seasonID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO seasons (id, media_id, season_number, name, overview, poster_path, air_date)
			VALUES (?, ?, ?, '', '', '', '')
			ON CONFLICT(media_id, season_number) DO NOTHING
		`, seasonID, mediaID, episode.SeasonNumber); err != nil {
			return nil, err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM seasons WHERE media_id = ? AND season_number = ?
		`, mediaID, episode.SeasonNumber).Scan(&seasonID); err != nil {
			return nil, err
		}

		episodeID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO episodes (
				id, season_id, source_id, episode_number, absolute_number,
				name, overview, still_path, air_date
			) VALUES (?, ?, ?, ?, ?, '', '', '', '')
			ON CONFLICT(season_id, episode_number) DO UPDATE SET
				source_id = CASE
					WHEN episodes.source_id IS NULL OR episodes.source_id = '' THEN excluded.source_id
					ELSE episodes.source_id
				END,
				absolute_number = excluded.absolute_number
		`, episodeID, seasonID, episode.SourceID, episode.EpisodeNumber, episode.AbsoluteNumber); err != nil {
			return nil, err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM episodes WHERE season_id = ? AND episode_number = ?
		`, seasonID, episode.EpisodeNumber).Scan(&episodeID); err != nil {
			return nil, err
		}
		ids = append(ids, episodeID)
	}
	return ids, nil
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
