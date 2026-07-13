package media

import (
	"context"
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

func newTestMediaService(t *testing.T) (*Service, *storage.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return NewService(NewRepository(db)), db
}
