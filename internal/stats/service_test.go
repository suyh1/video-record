package stats

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/media"
	"video-record/internal/records"
	"video-record/internal/storage"
)

func TestSummaryCoversViewingDimensionsAndIsolatesUsers(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	owner, err := authService.Initialize(ctx, "owner", "correct horse battery staple")
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	movie, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "统计电影",
	})
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE media_items SET runtime_minutes = 120 WHERE id = ?", movie.ID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO genres (source, source_id, name) VALUES ('tmdb', '18', '剧情');
		INSERT INTO media_genres (media_id, source, source_id) VALUES (?, 'tmdb', '18')
	`, movie.ID)
	require.NoError(t, err)

	recordService := records.NewService(records.NewRepository(db))
	rating := 85
	state, _, err := recordService.RecordStatus(ctx, records.RecordStatusInput{
		UpdateStateInput: records.UpdateStateInput{
			UserID: owner.ID, MediaID: movie.ID, Status: records.StatusCompleted,
			Rating: &rating, Source: records.SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC), ViewingMethod: "影院",
	})
	require.NoError(t, err)
	require.Equal(t, records.StatusCompleted, state.Status)
	_, err = recordService.AddRewatch(ctx, records.CreateWatchEventInput{
		UserID: owner.ID, MediaID: movie.ID,
		WatchedAt:     time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		ViewingMethod: "家庭电视", Source: records.SourceManual,
	})
	require.NoError(t, err)
	require.NoError(t, recordService.SetTags(ctx, owner.ID, movie.ID, []string{"经典"}))

	otherUserID := uuid.NewString()
	passwordHash, err := auth.HashPassword("another secure password")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, 'other', ?, 'member', 1, 0)
	`, otherUserID, passwordHash)
	require.NoError(t, err)
	_, _, err = recordService.RecordStatus(ctx, records.RecordStatusInput{
		UpdateStateInput: records.UpdateStateInput{
			UserID: otherUserID, MediaID: movie.ID, Status: records.StatusCompleted,
			Source: records.SourceManual, ExpectedVersion: 0,
		},
		WatchedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), ViewingMethod: "不应出现",
	})
	require.NoError(t, err)

	service := NewService(NewRepository(db))
	summary, err := service.Summary(ctx, owner.ID, "UTC")
	require.NoError(t, err)
	require.Equal(t, 2, summary.TotalWatches)
	require.Equal(t, 1, summary.UniqueMedia)
	require.Equal(t, 240, summary.TotalMinutes)
	require.Equal(t, 1, summary.RepeatWatches)
	require.Equal(t, 2, pointValue(summary.Monthly, "2026-07"))
	require.Equal(t, 2, pointValue(summary.Yearly, "2026"))
	require.Equal(t, 2, pointValue(summary.Genres, "剧情"))
	require.Equal(t, 1, pointValue(summary.Ratings, "8.0-8.9"))
	require.Equal(t, 1, pointValue(summary.Tags, "经典"))
	require.Equal(t, 1, pointValue(summary.ViewingMethods, "影院"))
	require.Equal(t, 1, pointValue(summary.ViewingMethods, "家庭电视"))
	require.Zero(t, pointValue(summary.ViewingMethods, "不应出现"))
}

func TestSummaryRejectsMissingUser(t *testing.T) {
	service := NewService(nil)
	_, err := service.Summary(context.Background(), "", "UTC")
	require.ErrorIs(t, err, ErrInvalidStatsQuery)
	_, err = service.Summary(context.Background(), "user", "Mars/Olympus")
	require.ErrorIs(t, err, ErrInvalidStatsQuery)
}

func pointValue(points []Point, label string) int {
	for _, point := range points {
		if point.Label == label {
			return point.Value
		}
	}
	return 0
}
