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
	require.Len(t, symbolSearch, 1)
	require.Equal(t, symbolItem.ID, symbolSearch[0].ID)
	require.Equal(t, StatusNone, symbolSearch[0].Status)
	require.Nil(t, symbolSearch[0].TMDBID)
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

func catalogIDs(items []CatalogItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
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
