package records

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateAcceptsExactlyFiveStatuses(t *testing.T) {
	for _, status := range []Status{StatusNone, StatusWishlist, StatusWatching, StatusCompleted, StatusDropped} {
		require.NoError(t, ValidateStatus(status))
	}
	require.ErrorIs(t, ValidateStatus("paused"), ErrInvalidStatus)
}

func TestRatingConvertsTenPointValuesToIntegerHundredScale(t *testing.T) {
	rating, err := RatingFromTen(8.7)
	require.NoError(t, err)
	require.Equal(t, 87, rating)
	require.Equal(t, 8.7, RatingToTen(rating))

	_, err = RatingFromTen(-0.1)
	require.ErrorIs(t, err, ErrInvalidRating)
	_, err = RatingFromTen(10.1)
	require.ErrorIs(t, err, ErrInvalidRating)
}

func TestSourcePriorityPreventsLowerPriorityOverwrite(t *testing.T) {
	require.True(t, CanOverwrite(SourceManual, SourceConfirmedImport))
	require.True(t, CanOverwrite(SourceConfirmedImport, SourceConfirmedSync))
	require.True(t, CanOverwrite(SourceConfirmedSync, SourceExternalDefault))
	require.True(t, CanOverwrite(SourceManual, SourceManual))
	require.False(t, CanOverwrite(SourceConfirmedSync, SourceManual))
	require.False(t, CanOverwrite(SourceExternalDefault, SourceConfirmedImport))
}
