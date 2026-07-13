package media

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/storage"
)

func TestUpsertExternalUsesLocalUUIDAndUniqueExternalIdentity(t *testing.T) {
	service, db := newTestMediaService(t)
	snapshot := ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie,
		Title: "降临", OriginalTitle: "Arrival", ReleaseDate: "2016-11-10",
	}

	first, err := service.UpsertExternal(context.Background(), snapshot)
	require.NoError(t, err)
	_, err = uuid.Parse(first.ID)
	require.NoError(t, err)
	require.NotEqual(t, snapshot.SourceID, first.ID)

	second, err := service.UpsertExternal(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	var items, identities int
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM media_items").Scan(&items))
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM media_external_ids").Scan(&identities))
	require.Equal(t, 1, items)
	require.Equal(t, 1, identities)
}

func TestExternalIdentityIncludesMediaType(t *testing.T) {
	service, _ := newTestMediaService(t)

	movie, err := service.UpsertExternal(context.Background(), ExternalSnapshot{
		Source: "tmdb", SourceID: "42", MediaType: MediaTypeMovie, Title: "Movie 42",
	})
	require.NoError(t, err)
	series, err := service.UpsertExternal(context.Background(), ExternalSnapshot{
		Source: "tmdb", SourceID: "42", MediaType: MediaTypeTV, Title: "Series 42",
	})
	require.NoError(t, err)

	require.NotEqual(t, movie.ID, series.ID)
}

func TestLinkAndRefreshExternalSnapshotPreservesCustomFields(t *testing.T) {
	service, _ := newTestMediaService(t)
	custom, err := service.CreateCustom(context.Background(), CreateCustomInput{
		MediaType: MediaTypeMovie,
		Title:     "我的译名",
		Overview:  "我的私人简介",
		Year:      "2016",
	})
	require.NoError(t, err)

	linked, err := service.LinkExternal(context.Background(), custom.ID, ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie,
		Title: "降临", OriginalTitle: "Arrival", Overview: "外部简介 v1", PosterPath: "/v1.jpg",
	})
	require.NoError(t, err)
	require.Equal(t, "我的译名", linked.Title)
	require.Equal(t, "我的私人简介", linked.Overview)
	require.Equal(t, "/v1.jpg", linked.PosterPath)

	refreshed, err := service.UpsertExternal(context.Background(), ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie,
		Title: "Arrival Updated", OriginalTitle: "Arrival", Overview: "外部简介 v2", PosterPath: "/v2.jpg",
	})
	require.NoError(t, err)
	require.Equal(t, custom.ID, refreshed.ID)
	require.Equal(t, "我的译名", refreshed.Title)
	require.Equal(t, "我的私人简介", refreshed.Overview)
	require.Equal(t, "Arrival Updated", refreshed.ExternalTitle)
	require.Equal(t, "外部简介 v2", refreshed.ExternalOverview)
	require.Equal(t, "/v2.jpg", refreshed.PosterPath)
}

func TestLinkExternalRejectsIdentityAlreadyOwnedByAnotherItem(t *testing.T) {
	service, _ := newTestMediaService(t)
	_, err := service.UpsertExternal(context.Background(), ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie, Title: "降临",
	})
	require.NoError(t, err)
	custom, err := service.CreateCustom(context.Background(), CreateCustomInput{
		MediaType: MediaTypeMovie, Title: "重复条目",
	})
	require.NoError(t, err)

	_, err = service.LinkExternal(context.Background(), custom.ID, ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie, Title: "降临",
	})

	require.ErrorIs(t, err, ErrExternalIdentityConflict)
}

func TestExternalRuntimeAndGenresRefreshAtomically(t *testing.T) {
	service, db := newTestMediaService(t)
	created, err := service.UpsertExternal(context.Background(), ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie,
		Title: "降临", RuntimeMinutes: 116,
		Genres: []ExternalGenre{{ID: "18", Name: "剧情"}, {ID: "878", Name: "科幻"}},
	})
	require.NoError(t, err)
	require.Equal(t, 116, created.RuntimeMinutes)
	require.Equal(t, []string{"剧情", "科幻"}, created.Genres)

	refreshed, err := service.UpsertExternal(context.Background(), ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: MediaTypeMovie,
		Title: "降临", RuntimeMinutes: 118,
		Genres: []ExternalGenre{{ID: "9648", Name: "悬疑"}},
	})
	require.NoError(t, err)
	require.Equal(t, created.ID, refreshed.ID)
	require.Equal(t, 118, refreshed.RuntimeMinutes)
	require.Equal(t, []string{"悬疑"}, refreshed.Genres)

	var associations int
	require.NoError(t, db.Reader().QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM media_genres WHERE media_id = ?
	`, created.ID).Scan(&associations))
	require.Equal(t, 1, associations)
}

func TestMediaServiceValidatesInputsAndReadsItems(t *testing.T) {
	service, _ := newTestMediaService(t)
	invalidSnapshots := []ExternalSnapshot{
		{SourceID: "1", MediaType: MediaTypeMovie, Title: "标题"},
		{Source: "tmdb", MediaType: MediaTypeMovie, Title: "标题"},
		{Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie},
		{Source: "tmdb", SourceID: "1", MediaType: "book", Title: "标题"},
		{Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie, Title: "标题", RuntimeMinutes: -1},
		{Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie, Title: "标题", Genres: []ExternalGenre{{Name: "剧情"}}},
		{Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie, Title: "标题", Genres: []ExternalGenre{{ID: "18"}}},
	}
	for _, snapshot := range invalidSnapshots {
		_, err := service.UpsertExternal(context.Background(), snapshot)
		require.ErrorIs(t, err, ErrInvalidMedia)
	}

	_, err := service.CreateCustom(context.Background(), CreateCustomInput{MediaType: "book", Title: "标题"})
	require.ErrorIs(t, err, ErrInvalidMedia)
	_, err = service.CreateCustom(context.Background(), CreateCustomInput{MediaType: MediaTypeMovie, Title: "  "})
	require.ErrorIs(t, err, ErrInvalidMedia)
	custom, err := service.CreateCustom(context.Background(), CreateCustomInput{
		MediaType: MediaTypeMovie, Title: "  本地条目  ",
	})
	require.NoError(t, err)
	require.Equal(t, "本地条目", custom.Title)
	require.Zero(t, custom.RuntimeMinutes)
	require.Empty(t, custom.Genres)

	found, err := service.FindByID(context.Background(), custom.ID)
	require.NoError(t, err)
	require.Equal(t, custom.ID, found.ID)
	_, err = service.FindByID(context.Background(), "missing")
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = service.LinkExternal(context.Background(), "", ExternalSnapshot{
		Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie, Title: "标题",
	})
	require.ErrorIs(t, err, ErrInvalidMedia)
}

func TestLinkExternalRejectsTypeMismatchAndRefreshesSameIdentity(t *testing.T) {
	service, _ := newTestMediaService(t)
	custom, err := service.CreateCustom(context.Background(), CreateCustomInput{
		MediaType: MediaTypeMovie, Title: "本地条目",
	})
	require.NoError(t, err)
	_, err = service.LinkExternal(context.Background(), custom.ID, ExternalSnapshot{
		Source: "tmdb", SourceID: "42", MediaType: MediaTypeTV, Title: "剧集",
	})
	require.ErrorIs(t, err, ErrMediaTypeMismatch)

	snapshot := ExternalSnapshot{
		Source: "tmdb", SourceID: "42", MediaType: MediaTypeMovie, Title: "电影",
		Genres: []ExternalGenre{{ID: "18", Name: "剧情"}},
	}
	linked, err := service.LinkExternal(context.Background(), custom.ID, snapshot)
	require.NoError(t, err)
	snapshot.Title = "电影更新"
	snapshot.Genres[0].Name = "剧情片"
	linkedAgain, err := service.LinkExternal(context.Background(), custom.ID, snapshot)
	require.NoError(t, err)
	require.Equal(t, linked.ID, linkedAgain.ID)
	require.Equal(t, "电影更新", linkedAgain.ExternalTitle)
	require.Equal(t, []string{"剧情片"}, linkedAgain.Genres)

	_, err = service.LinkExternal(context.Background(), "missing", snapshot)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestMediaRepositoryRollsBackGenreErrorsAndReportsClosedStorage(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	repository := NewRepository(db)
	duplicateGenres := ExternalSnapshot{
		Source: "tmdb", SourceID: "duplicate-genres", MediaType: MediaTypeMovie, Title: "测试",
		Genres: []ExternalGenre{{ID: "18", Name: "剧情"}, {ID: "18", Name: "剧情"}},
	}
	_, err = repository.UpsertExternal(ctx, duplicateGenres)
	require.Error(t, err)
	var items int
	require.NoError(t, db.Reader().QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items").Scan(&items))
	require.Zero(t, items)

	created, err := repository.CreateCustom(ctx, CreateCustomInput{MediaType: MediaTypeMovie, Title: "本地"})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	_, err = repository.UpsertExternal(ctx, ExternalSnapshot{
		Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie, Title: "外部",
	})
	require.Error(t, err)
	_, err = repository.LinkExternal(ctx, created.ID, ExternalSnapshot{
		Source: "tmdb", SourceID: "1", MediaType: MediaTypeMovie, Title: "外部",
	})
	require.Error(t, err)
	_, err = repository.CreateCustom(ctx, CreateCustomInput{MediaType: MediaTypeMovie, Title: "另一个"})
	require.Error(t, err)
	_, err = repository.FindByID(ctx, created.ID)
	require.Error(t, err)
}

func newTestMediaService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return NewService(NewRepository(db)), db
}
