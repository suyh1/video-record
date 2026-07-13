package records

import (
	"errors"
	"math"
)

type Status string

const (
	StatusNone      Status = "none"
	StatusWishlist  Status = "wishlist"
	StatusWatching  Status = "watching"
	StatusCompleted Status = "completed"
	StatusDropped   Status = "dropped"
)

type Source string

const (
	SourceExternalDefault Source = "external_default"
	SourceConfirmedSync   Source = "confirmed_sync"
	SourceConfirmedImport Source = "confirmed_import"
	SourceManual          Source = "manual"
)

var (
	ErrInvalidRating = errors.New("invalid rating")
	ErrInvalidStatus = errors.New("invalid status")
)

func ValidateStatus(status Status) error {
	switch status {
	case StatusNone, StatusWishlist, StatusWatching, StatusCompleted, StatusDropped:
		return nil
	default:
		return ErrInvalidStatus
	}
}

func RatingFromTen(value float64) (int, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 10 {
		return 0, ErrInvalidRating
	}
	return int(math.Round(value * 10)), nil
}

func RatingToTen(value int) float64 {
	return float64(value) / 10
}

func CanOverwrite(incoming, existing Source) bool {
	return sourcePriority(incoming) >= sourcePriority(existing)
}

func sourcePriority(source Source) int {
	switch source {
	case SourceManual:
		return 4
	case SourceConfirmedImport:
		return 3
	case SourceConfirmedSync:
		return 2
	case SourceExternalDefault:
		return 1
	default:
		return 0
	}
}
