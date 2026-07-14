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
	firstTime := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	firstMethod := "影院"
	state, err := recordService.UpdateRound(ctx, records.UpdateRoundInput{
		Scope:  records.RoundScope{UserID: owner.ID, MediaID: movie.ID},
		Status: records.StatusCompleted, Rating: &rating, RatingSet: true,
		ViewingMethod: &firstMethod, ViewingMethodSet: true,
		CompletedAt: &firstTime, Source: records.SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, records.StatusCompleted, state.Status)
	rewatch, err := recordService.StartRewatch(ctx, records.RewatchInput{
		Scope: records.RoundScope{UserID: owner.ID, MediaID: movie.ID}, ExpectedVersion: state.Version,
	})
	require.NoError(t, err)
	secondTime := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	secondMethod := "家庭电视"
	_, err = recordService.UpdateRound(ctx, records.UpdateRoundInput{
		Scope:  records.RoundScope{UserID: owner.ID, MediaID: movie.ID},
		Status: records.StatusCompleted, ViewingMethod: &secondMethod, ViewingMethodSet: true,
		CompletedAt: &secondTime, Source: records.SourceManual,
		ExpectedVersion: rewatch.Current.Version,
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
	otherTime := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	otherMethod := "不应出现"
	_, err = recordService.UpdateRound(ctx, records.UpdateRoundInput{
		Scope:  records.RoundScope{UserID: otherUserID, MediaID: movie.ID},
		Status: records.StatusCompleted, ViewingMethod: &otherMethod, ViewingMethodSet: true,
		CompletedAt: &otherTime, Source: records.SourceManual,
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

func TestArchivedStatsCountsEachViewingRoundOnce(t *testing.T) {
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
		MediaType: media.MediaTypeMovie, Title: "轮次统计电影",
	})
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, "UPDATE media_items SET runtime_minutes = 100 WHERE id = ?", movie.ID)
	require.NoError(t, err)
	recordService := records.NewService(records.NewRepository(db))
	rating := 86
	methodOne := "影院"
	firstTime := time.Date(2026, 7, 10, 12, 0, 1, 0, time.UTC)
	first, err := recordService.UpdateRound(ctx, records.UpdateRoundInput{
		Scope:  records.RoundScope{UserID: owner.ID, MediaID: movie.ID},
		Status: records.StatusCompleted, Rating: &rating, RatingSet: true,
		ViewingMethod: &methodOne, ViewingMethodSet: true,
		CompletedAt: &firstTime, Source: records.SourceManual,
	})
	require.NoError(t, err)
	rewatch, err := recordService.StartRewatch(ctx, records.RewatchInput{
		Scope: records.RoundScope{UserID: owner.ID, MediaID: movie.ID}, ExpectedVersion: first.Version,
	})
	require.NoError(t, err)
	methodTwo := "家庭电视"
	secondTime := firstTime.Add(24 * time.Hour)
	_, err = recordService.UpdateRound(ctx, records.UpdateRoundInput{
		Scope:  records.RoundScope{UserID: owner.ID, MediaID: movie.ID},
		Status: records.StatusCompleted, ViewingMethod: &methodTwo, ViewingMethodSet: true,
		CompletedAt: &secondTime, Source: records.SourceManual,
		ExpectedVersion: rewatch.Current.Version,
	})
	require.NoError(t, err)

	summary, err := NewService(NewRepository(db)).Summary(ctx, owner.ID, "UTC")
	require.NoError(t, err)
	require.Equal(t, 2, summary.TotalWatches)
	require.Equal(t, 1, summary.UniqueMedia)
	require.Equal(t, 200, summary.TotalMinutes)
	require.Equal(t, 1, summary.RepeatWatches)
	require.Equal(t, 1, pointValue(summary.Ratings, "8.0-8.9"))
	require.Equal(t, 1, pointValue(summary.ViewingMethods, methodOne))
	require.Equal(t, 1, pointValue(summary.ViewingMethods, methodTwo))
}

func pointValue(points []Point, label string) int {
	for _, point := range points {
		if point.Label == label {
			return point.Value
		}
	}
	return 0
}
