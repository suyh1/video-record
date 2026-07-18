package records

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/media"
)

func TestCatalogLibrarySearchAndUserIsolation(t *testing.T) {
	service, db, firstUserID, firstMediaID := newTestRecordsService(t)
	ctx := context.Background()
	mediaService := media.NewService(media.NewRepository(db))
	_, err := mediaService.LinkExternal(ctx, firstMediaID, media.ExternalSnapshot{
		Source: "tmdb", SourceID: "329865", MediaType: media.MediaTypeMovie, Title: "测试电影",
	})
	require.NoError(t, err)
	secondItem, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "漫长的季节", Year: "2023",
	})
	require.NoError(t, err)
	symbolItem, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "100%_电影", Year: "2026",
	})
	require.NoError(t, err)
	orphanItem, err := mediaService.UpsertExternal(ctx, media.ExternalSnapshot{
		Source: "tmdb", SourceID: "998877", MediaType: media.MediaTypeMovie, Title: "仅浏览的 TMDB 电影",
	})
	require.NoError(t, err)
	secondUserID := insertTestUser(t, db, "catalog-second")

	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: firstUserID, MediaID: firstMediaID}, Status: StatusCompleted,
		Source: SourceManual, ExpectedVersion: 0,
		CompletedAt: timePointer(time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	seasonOne := 1
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope:  RoundScope{UserID: firstUserID, MediaID: secondItem.ID, SeasonNumber: &seasonOne},
		Status: StatusWishlist, Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: secondUserID, MediaID: firstMediaID}, Status: StatusDropped,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	firstLibrary, err := service.Library(ctx, firstUserID, "")
	require.NoError(t, err)
	require.Len(t, firstLibrary, 2)
	completed, err := service.Library(ctx, firstUserID, StatusCompleted)
	require.NoError(t, err)
	require.Len(t, completed, 1)
	require.Equal(t, firstMediaID, completed[0].ID)
	require.Equal(t, StatusCompleted, completed[0].Status)
	require.NotNil(t, completed[0].TMDBID)
	require.Equal(t, 329865, *completed[0].TMDBID)

	firstSearch, err := service.SearchMedia(ctx, firstUserID, "测试电影")
	require.NoError(t, err)
	require.Len(t, firstSearch, 1)
	require.Equal(t, StatusCompleted, firstSearch[0].Status)
	secondSearch, err := service.SearchMedia(ctx, secondUserID, "测试电影")
	require.NoError(t, err)
	require.Len(t, secondSearch, 1)
	require.Equal(t, StatusDropped, secondSearch[0].Status)
	symbolSearch, err := service.SearchMedia(ctx, firstUserID, "%_")
	require.NoError(t, err)
	require.Empty(t, symbolSearch)
	orphanSearch, err := service.SearchMedia(ctx, firstUserID, "仅浏览的 TMDB 电影")
	require.NoError(t, err)
	require.Empty(t, orphanSearch)

	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: firstUserID, MediaID: symbolItem.ID}, Status: StatusWishlist,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: firstUserID, MediaID: orphanItem.ID}, Status: StatusWishlist,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	symbolSearch, err = service.SearchMedia(ctx, firstUserID, "%_")
	require.NoError(t, err)
	require.Len(t, symbolSearch, 1)
	require.Equal(t, symbolItem.ID, symbolSearch[0].ID)
	require.Equal(t, StatusWishlist, symbolSearch[0].Status)
	require.Nil(t, symbolSearch[0].TMDBID)
	orphanSearch, err = service.SearchMedia(ctx, firstUserID, "仅浏览的 TMDB 电影")
	require.NoError(t, err)
	require.Len(t, orphanSearch, 1)
	require.Equal(t, orphanItem.ID, orphanSearch[0].ID)
	require.NotNil(t, orphanSearch[0].TMDBID)
	require.Equal(t, 998877, *orphanSearch[0].TMDBID)
}

func TestMediaProfileProjectsWatchingThenLatestSeasonStatus(t *testing.T) {
	service, db, userID, _ := newTestRecordsService(t)
	mediaService := media.NewService(media.NewRepository(db))
	series, err := mediaService.CreateCustom(context.Background(), media.CreateCustomInput{
		MediaType: media.MediaTypeTV, Title: "季级投影",
	})
	require.NoError(t, err)
	seasonOne, seasonTwo := 1, 2
	completedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	_, err = service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: series.ID, SeasonNumber: &seasonOne},
		Status: StatusCompleted, CompletedAt: &completedAt, Source: SourceManual,
	})
	require.NoError(t, err)
	second, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: series.ID, SeasonNumber: &seasonTwo},
		Status: StatusWatching, Source: SourceManual,
	})
	require.NoError(t, err)

	watching, err := service.Library(context.Background(), userID, StatusWatching)
	require.NoError(t, err)
	require.Equal(t, []string{series.ID}, catalogIDs(watching))

	_, err = service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:  RoundScope{UserID: userID, MediaID: series.ID, SeasonNumber: &seasonTwo},
		Status: StatusDropped, Source: SourceManual, ExpectedVersion: second.Version,
	})
	require.NoError(t, err)
	dropped, err := service.Library(context.Background(), userID, StatusDropped)
	require.NoError(t, err)
	require.Equal(t, []string{series.ID}, catalogIDs(dropped))

	search, err := service.SearchMedia(context.Background(), userID, "季级投影")
	require.NoError(t, err)
	require.Len(t, search, 1)
	require.Equal(t, StatusDropped, search[0].Status)
}

func TestStateReadsProjectedProfileAndCurrentRoundAfterViewingMigration(t *testing.T) {
	service, _, userID, movieID := newTestRecordsService(t)
	rating := 87
	note := "归档前的私人笔记"
	completedAt := time.Date(2026, 7, 13, 12, 30, 45, 0, time.UTC)
	round, err := service.UpdateRound(context.Background(), UpdateRoundInput{
		Scope:       RoundScope{UserID: userID, MediaID: movieID},
		Status:      StatusCompleted,
		Rating:      &rating,
		RatingSet:   true,
		Note:        &note,
		NoteSet:     true,
		CompletedAt: &completedAt,
		Source:      SourceManual,
	})
	require.NoError(t, err)

	state, exists, err := service.State(context.Background(), userID, movieID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, StatusCompleted, state.Status)
	require.Equal(t, round.ProfileVersion, state.Version)
	require.Equal(t, rating, *state.Rating)
	require.Equal(t, note, *state.Note)
	require.Equal(t, completedAt, *state.CompletedAt)
}

func catalogIDs(items []CatalogItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestCollectionItemsReturnInPositionOrderIndependentOfLibraryPage(t *testing.T) {
	service, db, userID, firstMediaID := newTestRecordsService(t)
	ctx := context.Background()
	mediaService := media.NewService(media.NewRepository(db))

	// Create many library items so firstMediaID would fall off a small library page.
	for i := 0; i < 5; i++ {
		item, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
			MediaType: media.MediaTypeMovie, Title: "填充" + string(rune('A'+i)),
		})
		require.NoError(t, err)
		_, err = service.UpdateRound(ctx, UpdateRoundInput{
			Scope: RoundScope{UserID: userID, MediaID: item.ID}, Status: StatusWishlist,
			Source: SourceManual, ExpectedVersion: 0,
		})
		require.NoError(t, err)
	}
	// firstMediaID is older; mark it completed and put it only in a collection.
	_, err := service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: userID, MediaID: firstMediaID}, Status: StatusCompleted,
		Source: SourceManual, ExpectedVersion: 0,
		CompletedAt: timePointer(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	second, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
		MediaType: media.MediaTypeMovie, Title: "片单第二部",
	})
	require.NoError(t, err)
	_, err = service.UpdateRound(ctx, UpdateRoundInput{
		Scope: RoundScope{UserID: userID, MediaID: second.ID}, Status: StatusWishlist,
		Source: SourceManual, ExpectedVersion: 0,
	})
	require.NoError(t, err)

	collection, err := service.CreateCollection(ctx, userID, "周末")
	require.NoError(t, err)
	require.NoError(t, service.AddCollectionItem(ctx, userID, collection.ID, second.ID))
	require.NoError(t, service.AddCollectionItem(ctx, userID, collection.ID, firstMediaID))
	// Reorder: firstMediaID then second.
	require.NoError(t, service.ReplaceCollectionItems(ctx, userID, collection.ID, []string{firstMediaID, second.ID}))

	items, err := service.CollectionItems(ctx, userID, collection.ID, "")
	require.NoError(t, err)
	require.Equal(t, []string{firstMediaID, second.ID}, catalogIDs(items))
	require.Equal(t, StatusCompleted, items[0].Status)
	require.Equal(t, StatusWishlist, items[1].Status)

	// Status filter reserved for Task 10 — empty status returns full collection.
	_, err = service.CollectionItems(ctx, userID, "missing", "")
	require.ErrorIs(t, err, ErrCollectionNotFound)
}

func TestCatalogValidatesCurrentUserStatusAndQuery(t *testing.T) {
	service, _, userID, _ := newTestRecordsService(t)
	ctx := context.Background()
	_, _, err := service.State(ctx, "", "media")
	require.ErrorIs(t, err, ErrInvalidRecord)
	_, err = service.Library(ctx, "", "")
	require.ErrorIs(t, err, ErrInvalidRecord)
	_, err = service.Library(ctx, userID, "paused")
	require.ErrorIs(t, err, ErrInvalidRecord)
	_, err = service.SearchMedia(ctx, userID, "  ")
	require.ErrorIs(t, err, ErrInvalidRecord)
}

func TestLibraryPageCursorPagination(t *testing.T) {
	service, db, userID, firstMediaID := newTestRecordsService(t)
	ctx := context.Background()
	mediaService := media.NewService(media.NewRepository(db))

	ids := []string{firstMediaID}
	for i := 0; i < 4; i++ {
		item, err := mediaService.CreateCustom(ctx, media.CreateCustomInput{
			MediaType: media.MediaTypeMovie, Title: "分页电影" + string(rune('A'+i)), Year: "2026",
		})
		require.NoError(t, err)
		ids = append(ids, item.ID)
	}
	// Ensure distinct updated_at ordering: mark newest last.
	for index, mediaID := range ids {
		_, err := service.UpdateRound(ctx, UpdateRoundInput{
			Scope: RoundScope{UserID: userID, MediaID: mediaID}, Status: StatusWishlist,
			Source: SourceManual, ExpectedVersion: 0,
		})
		require.NoError(t, err)
		// Bump profile timestamps in order via sequential updates.
		if index < len(ids)-1 {
			time.Sleep(2 * time.Millisecond)
		}
	}

	firstPage, err := service.LibraryPage(ctx, userID, LibraryQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, firstPage.Items, 2)
	require.NotEmpty(t, firstPage.NextCursor)

	secondPage, err := service.LibraryPage(ctx, userID, LibraryQuery{Limit: 2, Cursor: firstPage.NextCursor})
	require.NoError(t, err)
	require.Len(t, secondPage.Items, 2)
	require.NotEmpty(t, secondPage.NextCursor)

	thirdPage, err := service.LibraryPage(ctx, userID, LibraryQuery{Limit: 2, Cursor: secondPage.NextCursor})
	require.NoError(t, err)
	require.Len(t, thirdPage.Items, 1)
	require.Empty(t, thirdPage.NextCursor)

	seen := map[string]struct{}{}
	for _, page := range []LibraryPage{firstPage, secondPage, thirdPage} {
		for _, item := range page.Items {
			_, exists := seen[item.ID]
			require.False(t, exists, "duplicate media %s across pages", item.ID)
			seen[item.ID] = struct{}{}
		}
	}
	require.Len(t, seen, 5)

	_, err = service.LibraryPage(ctx, userID, LibraryQuery{Cursor: "not-a-cursor", Limit: 2})
	require.ErrorIs(t, err, ErrInvalidRecord)

	_, err = service.LibraryPage(ctx, userID, LibraryQuery{Limit: 101})
	require.ErrorIs(t, err, ErrInvalidRecord)

	// Backward-compatible Library still returns a page worth of items.
	all, err := service.Library(ctx, userID, "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all), 5)
}
