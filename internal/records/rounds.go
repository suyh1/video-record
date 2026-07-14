package records

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidRoundScope = errors.New("invalid round scope")
	ErrInvalidWatchedAt  = errors.New("invalid watched at")
	ErrRoundArchived     = errors.New("round archived")
)

type RoundScope struct {
	UserID       string
	MediaID      string
	SeasonNumber *int
}

type WatchRound struct {
	ID             string
	UserID         string
	MediaID        string
	SeasonNumber   *int
	RoundNumber    int
	Status         Status
	Rating         *int
	Note           *string
	ViewingMethod  *string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	ArchivedAt     *time.Time
	Version        int
	StatusSource   Source
	RatingSource   Source
	NoteSource     Source
	ProfileVersion int
}

type UpdateRoundInput struct {
	Scope            RoundScope
	Status           Status
	Rating           *int
	RatingSet        bool
	Note             *string
	NoteSet          bool
	ViewingMethod    *string
	ViewingMethodSet bool
	CompletedAt      *time.Time
	Source           Source
	ExpectedVersion  int
}

func (service *Service) CurrentRound(ctx context.Context, scope RoundScope) (WatchRound, error) {
	if err := service.repository.ValidateRoundScope(ctx, scope); err != nil {
		return WatchRound{}, err
	}
	current, exists, err := service.repository.FindCurrentRound(ctx, scope)
	if err != nil {
		return WatchRound{}, err
	}
	if exists {
		return service.attachProfileVersion(ctx, current)
	}
	return service.attachProfileVersion(ctx, emptyRound(scope))
}

func (service *Service) UpdateRound(ctx context.Context, input UpdateRoundInput) (WatchRound, error) {
	if err := service.repository.ValidateRoundScope(ctx, input.Scope); err != nil {
		return WatchRound{}, err
	}
	if ValidateStatus(input.Status) != nil || sourcePriority(input.Source) == 0 || input.ExpectedVersion < 0 {
		return WatchRound{}, ErrInvalidRecord
	}
	if input.RatingSet && input.Rating != nil && (*input.Rating < 0 || *input.Rating > 100) {
		return WatchRound{}, ErrInvalidRating
	}
	now := service.now().UTC()
	if input.CompletedAt != nil && input.CompletedAt.After(now) {
		return WatchRound{}, ErrInvalidWatchedAt
	}

	current, exists, err := service.repository.FindCurrentRound(ctx, input.Scope)
	if err != nil {
		return WatchRound{}, err
	}
	if !exists && input.ExpectedVersion > 0 {
		latest, latestExists, latestErr := service.repository.FindLatestRound(ctx, input.Scope)
		if latestErr != nil {
			return WatchRound{}, latestErr
		}
		if latestExists && latest.ArchivedAt != nil {
			return latest, ErrRoundArchived
		}
	}
	if !exists {
		current = emptyRound(input.Scope)
		current.StatusSource = input.Source
		current.RatingSource = input.Source
		current.NoteSource = input.Source
	}
	if current.ArchivedAt != nil {
		return current, ErrRoundArchived
	}
	if current.Version != input.ExpectedVersion {
		return current, ErrVersionConflict
	}

	next := current
	changed := !exists
	if !exists || CanOverwrite(input.Source, current.StatusSource) {
		changed = changed || next.Status != input.Status || next.StatusSource != input.Source
		next.Status, next.StatusSource = input.Status, input.Source
	}
	if input.RatingSet && (!exists || CanOverwrite(input.Source, current.RatingSource)) {
		changed = changed || !equalIntPointers(next.Rating, input.Rating) || next.RatingSource != input.Source
		next.Rating, next.RatingSource = cloneIntPointer(input.Rating), input.Source
	}
	if input.NoteSet && (!exists || CanOverwrite(input.Source, current.NoteSource)) {
		changed = changed || !equalStringPointers(next.Note, input.Note) || next.NoteSource != input.Source
		next.Note, next.NoteSource = cloneStringPointer(input.Note), input.Source
	}
	if input.ViewingMethodSet {
		changed = changed || !equalStringPointers(next.ViewingMethod, input.ViewingMethod)
		next.ViewingMethod = cloneStringPointer(input.ViewingMethod)
	}
	if next.Status == StatusWatching && next.StartedAt == nil {
		next.StartedAt = timePointerCopy(now)
		changed = true
	}
	if next.Status == StatusCompleted {
		if input.CompletedAt == nil {
			return current, ErrInvalidWatchedAt
		}
		completed := input.CompletedAt.UTC()
		changed = changed || next.CompletedAt == nil || !next.CompletedAt.Equal(completed)
		next.CompletedAt = &completed
	} else if next.CompletedAt != nil {
		next.CompletedAt = nil
		changed = true
	}
	if !changed {
		return current, nil
	}

	next.Version = current.Version + 1
	if !exists {
		next.ID = uuid.NewString()
		next.RoundNumber = 1
		if err := service.repository.InsertRound(ctx, next); err != nil {
			return WatchRound{}, err
		}
		return service.attachProfileVersion(ctx, next)
	}
	updated, err := service.repository.UpdateRound(ctx, next, current.Version)
	if err != nil {
		return WatchRound{}, err
	}
	if !updated {
		return current, ErrVersionConflict
	}
	return service.attachProfileVersion(ctx, next)
}

func (service *Service) attachProfileVersion(ctx context.Context, round WatchRound) (WatchRound, error) {
	profile, exists, err := service.repository.FindProfile(ctx, round.UserID, round.MediaID)
	if err != nil {
		return WatchRound{}, err
	}
	if exists {
		round.ProfileVersion = profile.Version
	}
	return round, nil
}

func emptyRound(scope RoundScope) WatchRound {
	return WatchRound{
		UserID: scope.UserID, MediaID: scope.MediaID,
		SeasonNumber: cloneIntPointer(scope.SeasonNumber),
		RoundNumber:  1, Status: StatusNone,
	}
}

func timePointerCopy(value time.Time) *time.Time {
	copy := value
	return &copy
}
