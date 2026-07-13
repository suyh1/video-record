package records

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"video-record/internal/auth"
	"video-record/internal/media"
	"video-record/internal/storage"
)

func TestStateUpdateUsesOptimisticVersionAndManualPriority(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	rating := 91
	note := "手工笔记"

	manual, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusCompleted,
		Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 1, manual.Version)
	require.Equal(t, StatusCompleted, manual.Status)
	require.Equal(t, 91, *manual.Rating)
	require.Equal(t, note, *manual.Note)

	_, err = service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWishlist,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.ErrorIs(t, err, ErrVersionConflict)

	syncRating := 40
	syncNote := "同步值"
	afterSync, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWatching,
		Rating: &syncRating, Note: &syncNote, Source: SourceConfirmedSync, ExpectedVersion: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, afterSync.Version)
	require.Equal(t, StatusCompleted, afterSync.Status)
	require.Equal(t, 91, *afterSync.Rating)
	require.Equal(t, note, *afterSync.Note)
}

func TestStateUpdateClearsExplicitNullableFields(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	rating := 91
	note := "手工笔记"
	created, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusCompleted,
		Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	cleared, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusCompleted,
		RatingSet: true, NoteSet: true, Source: SourceManual, ExpectedVersion: created.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 2, cleared.Version)
	require.Nil(t, cleared.Rating)
	require.Nil(t, cleared.Note)

	persisted, exists, err := service.repository.FindState(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, cleared.Version, persisted.Version)
	require.Nil(t, persisted.Rating)
	require.Nil(t, persisted.Note)
}

func TestStateUpdatePreservesOmittedFieldsAndSkipsNoopVersion(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	rating := 84
	note := "保留内容"
	created, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWishlist,
		Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	updated, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWatching,
		Source: SourceManual, ExpectedVersion: created.Version,
	})
	require.NoError(t, err)
	require.Equal(t, 2, updated.Version)
	require.Equal(t, rating, *updated.Rating)
	require.Equal(t, note, *updated.Note)

	unchanged, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWatching,
		Rating: &rating, Note: &note, Source: SourceManual, ExpectedVersion: updated.Version,
	})
	require.NoError(t, err)
	require.Equal(t, updated.Version, unchanged.Version)
}

func TestStateUpdateRejectsInvalidInput(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	invalidRating := 101
	tests := []UpdateStateInput{
		{MediaID: mediaID, Status: StatusWishlist, Source: SourceManual},
		{UserID: userID, Status: StatusWishlist, Source: SourceManual},
		{UserID: userID, MediaID: mediaID, Status: "paused", Source: SourceManual},
		{UserID: userID, MediaID: mediaID, Status: StatusWishlist, Source: "unknown"},
	}
	for _, input := range tests {
		_, err := service.UpdateState(context.Background(), input)
		require.ErrorIs(t, err, ErrInvalidRecord)
	}
	_, err := service.UpdateState(context.Background(), UpdateStateInput{
		UserID: userID, MediaID: mediaID, Status: StatusWishlist,
		Rating: &invalidRating, Source: SourceManual,
	})
	require.ErrorIs(t, err, ErrInvalidRating)
}

func TestTagsAndCollectionsArePrivateToTheirOwner(t *testing.T) {
	service, db, firstUserID, mediaID := newTestRecordsService(t)
	secondUserID := insertTestUser(t, db, "second")

	require.NoError(t, service.SetTags(context.Background(), firstUserID, mediaID, []string{"科幻", "家庭"}))
	require.NoError(t, service.SetTags(context.Background(), secondUserID, mediaID, []string{"科幻"}))
	firstTags, err := service.Tags(context.Background(), firstUserID, mediaID)
	require.NoError(t, err)
	require.Equal(t, []string{"家庭", "科幻"}, firstTags)
	secondTags, err := service.Tags(context.Background(), secondUserID, mediaID)
	require.NoError(t, err)
	require.Equal(t, []string{"科幻"}, secondTags)

	collection, err := service.CreateCollection(context.Background(), firstUserID, "周末电影")
	require.NoError(t, err)
	require.NoError(t, service.AddCollectionItem(context.Background(), firstUserID, collection.ID, mediaID))
	firstCollections, err := service.Collections(context.Background(), firstUserID)
	require.NoError(t, err)
	require.Len(t, firstCollections, 1)
	require.Equal(t, mediaID, firstCollections[0].Items[0])
	secondCollections, err := service.Collections(context.Background(), secondUserID)
	require.NoError(t, err)
	require.Empty(t, secondCollections)
	require.ErrorIs(
		t,
		service.AddCollectionItem(context.Background(), secondUserID, collection.ID, mediaID),
		ErrCollectionNotFound,
	)
}

func TestTagsAndCollectionsValidateRequiredFields(t *testing.T) {
	service, _, userID, mediaID := newTestRecordsService(t)
	require.ErrorIs(t, service.SetTags(context.Background(), "", mediaID, nil), ErrInvalidRecord)
	require.ErrorIs(t, service.SetTags(context.Background(), userID, "", nil), ErrInvalidRecord)

	_, err := service.CreateCollection(context.Background(), userID, "  ")
	require.ErrorIs(t, err, ErrInvalidRecord)
	collection, err := service.CreateCollection(context.Background(), userID, "  周末电影  ")
	require.NoError(t, err)
	require.Equal(t, "周末电影", collection.Name)
}

func newTestRecordsService(t *testing.T) (*Service, *storage.DB, string, string) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(context.Background(), db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	authService := auth.NewService(auth.NewRepository(db), auth.ServiceOptions{})
	user, err := authService.Initialize(context.Background(), "owner", "correct horse battery staple")
	require.NoError(t, err)
	mediaService := media.NewService(media.NewRepository(db))
	item, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "测试电影",
	})
	require.NoError(t, err)
	return NewService(NewRepository(db)), db, user.ID, item.ID
}

func insertTestUser(t *testing.T, db *storage.DB, username string) string {
	t.Helper()
	id := uuid.NewString()
	passwordHash, err := auth.HashPassword("another secure password")
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(context.Background(), `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, ?, ?, 'member', 1, 0)
	`, id, username, passwordHash)
	require.NoError(t, err)
	return id
}
