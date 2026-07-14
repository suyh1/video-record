package testutil_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/records"
	"video-record/internal/storage"
	"video-record/internal/testutil"
)

func TestSeedCreatesExactSyntheticDataset(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	result, err := testutil.Seed(ctx, db, testutil.SeedOptions{
		Users: 3, MediaItems: 20, WatchEvents: 80,
		Password: stringsJoin("Synthetic", "seed", "password"),
		Now:      time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, result.UserIDs, 3)
	require.Len(t, result.MediaIDs, 20)
	require.Equal(t, "perf-user-0", result.Username)

	for table, expected := range map[string]int{
		"users": 3, "media_items": 20, "media_external_ids": 20,
		"user_media_profiles": 20, "watch_rounds": 20,
		"watch_events": 80, "watch_event_participants": 80,
	} {
		var count int
		require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Equal(t, expected, count, table)
	}

	calendar, err := records.NewService(records.NewRepository(db)).CalendarMonth(ctx, records.CalendarMonthInput{
		UserID: result.UserIDs[0], Year: 2026, Month: time.July,
		Timezone: "UTC", Filter: records.CalendarFilterAll,
	})
	require.NoError(t, err)
	require.Len(t, calendar.Events, 80)
}

func TestSeedRejectsInvalidOptionsBeforeWriting(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	valid := testutil.SeedOptions{
		Users: 1, MediaItems: 1, WatchEvents: 1,
		Password: stringsJoin("Synthetic", "seed", "password"),
		Now:      now,
	}
	tests := map[string]testutil.SeedOptions{
		"no users":          withSeedCounts(valid, 0, 1, 1),
		"negative media":    withSeedCounts(valid, 1, -1, 0),
		"negative events":   withSeedCounts(valid, 1, 1, -1),
		"events need media": withSeedCounts(valid, 1, 0, 1),
		"short password":    {Users: 1, MediaItems: 1, Password: "too-short", Now: now},
	}
	for name, options := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := testutil.Seed(context.Background(), nil, options)
			require.True(t, errors.Is(err, testutil.ErrInvalidSeedOptions), err)
		})
	}
}

func TestSeedRollsBackEveryTableWhenAnInsertFails(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, created_at, updated_at
		) VALUES ('perf-media-00010', 'movie', 'Existing', 'Existing', '', '', '', '', 0, 0)
	`)
	require.NoError(t, err)

	_, err = testutil.Seed(ctx, db, testutil.SeedOptions{
		Users: 2, MediaItems: 20, WatchEvents: 0,
		Password: stringsJoin("Synthetic", "rollback", "password"),
		Now:      time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)

	for table, expected := range map[string]int{
		"users": 0, "media_items": 1, "media_external_ids": 0,
		"user_media_profiles": 0, "watch_rounds": 0,
	} {
		var count int
		require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Equal(t, expected, count, table)
	}
}

func withSeedCounts(options testutil.SeedOptions, users, mediaItems, watchEvents int) testutil.SeedOptions {
	options.Users = users
	options.MediaItems = mediaItems
	options.WatchEvents = watchEvents
	return options
}

func stringsJoin(parts ...string) string {
	result := ""
	for index, part := range parts {
		if index > 0 {
			result += "-"
		}
		result += part
	}
	return result
}
