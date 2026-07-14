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

func TestTagsAndCollectionsArePrivateToTheirOwner(t *testing.T) {
	service, db, firstUserID, mediaID := newTestRecordsService(t)
	secondUserID := insertTestUser(t, db, "second")

	require.NoError(t, service.SetTags(context.Background(), firstUserID, mediaID, []string{" 科幻 ", "家庭", "科幻", ""}))
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

func TestCollectionItemsCanBeReorderedAndRemovedOnlyByTheirOwner(t *testing.T) {
	ctx := context.Background()
	service, db, ownerID, firstMediaID := newTestRecordsService(t)
	mediaService := media.NewService(media.NewRepository(db))
	secondMedia, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "第二部测试电影",
	})
	require.NoError(t, err)
	thirdMedia, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "第三部测试电影",
	})
	require.NoError(t, err)
	collection, err := service.CreateCollection(ctx, ownerID, "排序片单")
	require.NoError(t, err)
	for _, mediaID := range []string{firstMediaID, secondMedia.ID, thirdMedia.ID} {
		require.NoError(t, service.AddCollectionItem(ctx, ownerID, collection.ID, mediaID))
	}

	require.NoError(t, service.ReplaceCollectionItems(
		ctx, ownerID, collection.ID, []string{thirdMedia.ID, firstMediaID},
	))
	collections, err := service.Collections(ctx, ownerID)
	require.NoError(t, err)
	require.Equal(t, []string{thirdMedia.ID, firstMediaID}, collections[0].Items)

	otherUserID := insertTestUser(t, db, "collection-reorder-other")
	require.ErrorIs(t,
		service.ReplaceCollectionItems(ctx, otherUserID, collection.ID, []string{firstMediaID}),
		ErrCollectionNotFound,
	)
	require.ErrorIs(t,
		service.ReplaceCollectionItems(ctx, ownerID, collection.ID, []string{firstMediaID, firstMediaID}),
		ErrInvalidRecord,
	)
	require.ErrorIs(t,
		service.ReplaceCollectionItems(ctx, ownerID, collection.ID, []string{"missing-media"}),
		ErrInvalidRecord,
	)
	collections, err = service.Collections(ctx, ownerID)
	require.NoError(t, err)
	require.Equal(t, []string{thirdMedia.ID, firstMediaID}, collections[0].Items)
}

func TestVersionedTagsAdvanceStateAndRejectStaleWriters(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)

	updated, err := service.SetTagsVersioned(
		context.Background(), userID, mediaID, []string{" 科幻 ", "家庭", "科幻"}, 0,
	)
	require.NoError(t, err)
	require.Equal(t, 1, updated.Version)
	tags, err := service.Tags(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.Equal(t, []string{"家庭", "科幻"}, tags)

	stale, err := service.SetTagsVersioned(
		context.Background(), userID, mediaID, []string{"覆盖失败"}, 0,
	)
	require.ErrorIs(t, err, ErrVersionConflict)
	require.Equal(t, updated.Version, stale.Version)
	tags, err = service.Tags(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.Equal(t, []string{"家庭", "科幻"}, tags)
	advanced, err := service.SetTagsVersioned(
		context.Background(), userID, mediaID, []string{"重看"}, updated.Version,
	)
	require.NoError(t, err)
	require.Equal(t, 2, advanced.Version)
	tags, err = service.Tags(context.Background(), userID, mediaID)
	require.NoError(t, err)
	require.Equal(t, []string{"重看"}, tags)

	secondUserID := insertTestUser(t, db, "versioned-tags-second")
	missing, err := service.SetTagsVersioned(context.Background(), secondUserID, mediaID, nil, 0)
	require.NoError(t, err)
	require.Equal(t, 1, missing.Version)
	_, err = service.SetTagsVersioned(context.Background(), "", mediaID, nil, 0)
	require.ErrorIs(t, err, ErrInvalidRecord)
	_, err = service.SetTagsVersioned(context.Background(), userID, mediaID, nil, -1)
	require.ErrorIs(t, err, ErrInvalidRecord)

	repositoryUpdated, err := service.repository.SetTagsVersioned(
		context.Background(), userID, mediaID, []string{"覆盖失败"}, 0,
	)
	require.NoError(t, err)
	require.False(t, repositoryUpdated)
}

func TestRecordPointerHelpersCompareAndCloneNullableValues(t *testing.T) {
	first, same, other := 1, 1, 2
	require.True(t, equalIntPointers(nil, nil))
	require.True(t, equalIntPointers(&first, &same))
	require.False(t, equalIntPointers(&first, nil))
	require.False(t, equalIntPointers(&first, &other))
	require.True(t, equalStringPointers(nil, nil))
	require.Nil(t, cloneIntPointer(nil))
	require.Nil(t, cloneStringPointer(nil))
}

func TestRecordsServicesReturnStorageErrorsAfterDatabaseCloses(t *testing.T) {
	service, db, userID, mediaID := newTestRecordsService(t)
	ctx := context.Background()
	require.NoError(t, db.Close())

	_, err := service.CurrentRound(ctx, RoundScope{UserID: userID, MediaID: mediaID})
	require.Error(t, err)
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: userID, MediaID: mediaID}, Status: StatusWatching,
		Source: SourceManual,
	})
	require.Error(t, err)
	_, err = service.RoundHistory(ctx, RoundScope{UserID: userID, MediaID: mediaID})
	require.Error(t, err)
	_, err = service.RoundDetail(ctx, RoundScope{UserID: userID, MediaID: mediaID}, "round-id")
	require.Error(t, err)
	_, err = service.WatchEvents(ctx, userID, mediaID)
	require.Error(t, err)
	_, err = service.ExportData(ctx, userID, ExportFormatJSON)
	require.Error(t, err)
	require.Error(t, service.SetTags(ctx, userID, mediaID, []string{"离线"}))
	_, err = service.SetTagsVersioned(ctx, userID, mediaID, []string{"离线"}, 0)
	require.Error(t, err)
	_, err = service.Tags(ctx, userID, mediaID)
	require.Error(t, err)
	_, err = service.CreateCollection(ctx, userID, "离线片单")
	require.Error(t, err)
	require.Error(t, service.AddCollectionItem(ctx, userID, "collection-id", mediaID))
	require.Error(t, service.ReplaceCollectionItems(ctx, userID, "collection-id", []string{mediaID}))
	_, err = service.Collections(ctx, userID)
	require.Error(t, err)
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

	for _, input := range []struct {
		userID       string
		collectionID string
		mediaID      string
	}{
		{"", collection.ID, mediaID},
		{userID, "", mediaID},
		{userID, collection.ID, ""},
	} {
		require.ErrorIs(t,
			service.AddCollectionItem(context.Background(), input.userID, input.collectionID, input.mediaID),
			ErrInvalidRecord,
		)
	}
	require.ErrorIs(t,
		service.ReplaceCollectionItems(context.Background(), "", collection.ID, nil),
		ErrInvalidRecord,
	)
	require.ErrorIs(t,
		service.ReplaceCollectionItems(context.Background(), userID, "", nil),
		ErrInvalidRecord,
	)
	require.ErrorIs(t,
		service.ReplaceCollectionItems(context.Background(), userID, collection.ID, []string{" "}),
		ErrInvalidRecord,
	)
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
