package sync

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/storage"
)

func TestCandidateQueriesStatusConfirmIgnoreAndResolutionBoundaries(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "confirm-target", "movie", "Confirm Candidate", "2024")
	insertCandidateMedia(t, db, "other-target", "movie", "Other Candidate", "2024")
	confirmCandidate, err := service.Ingest(ctx, accountID, candidateMovieEvent(
		"event-confirm", "provider-confirm", "Confirm Candidate", 2024,
	))
	require.NoError(t, err)
	require.Equal(t, CandidatePossible, confirmCandidate.Status)
	unmatched, err := service.Ingest(ctx, accountID, candidateMovieEvent(
		"event-unmatched", "provider-unmatched", "No Local Match", 1999,
	))
	require.NoError(t, err)
	require.Equal(t, CandidateUnmatched, unmatched.Status)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO sync_runs (
			id, account_id, job_kind, status, cursor, summary_json, started_at, finished_at
		) VALUES ('latest-run', ?, 'incremental', 'succeeded', 'cursor', '{}', ?, ?)
	`, accountID, time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC).UnixMilli(),
		time.Date(2026, 7, 13, 11, 5, 0, 0, time.UTC).UnixMilli())
	require.NoError(t, err)

	possible, err := service.Candidates(ctx, userID, CandidatePossible)
	require.NoError(t, err)
	require.Len(t, possible, 1)
	require.Equal(t, confirmCandidate.ID, possible[0].ID)
	status, err := service.Status(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 2, status.PendingTotal)
	require.Len(t, status.Accounts, 1)
	require.Equal(t, "succeeded", status.Accounts[0].LastRunStatus)
	require.Equal(t, time.Date(2026, 7, 13, 11, 5, 0, 0, time.UTC), *status.Accounts[0].LastRunAt)

	confirmed, err := service.Confirm(ctx, userID, confirmCandidate.ID)
	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, confirmed.Status)
	require.Equal(t, "confirm-target", confirmed.MediaID)
	replayed, err := service.Confirm(ctx, userID, confirmCandidate.ID)
	require.NoError(t, err)
	require.Equal(t, confirmed.ID, replayed.ID)
	replayed, err = service.Rematch(ctx, userID, confirmCandidate.ID, "confirm-target", "")
	require.NoError(t, err)
	require.Equal(t, confirmed.ID, replayed.ID)
	_, err = service.Rematch(ctx, userID, confirmCandidate.ID, "other-target", "")
	require.ErrorIs(t, err, ErrCandidateResolved)
	_, err = service.Ignore(ctx, userID, confirmCandidate.ID)
	require.ErrorIs(t, err, ErrCandidateResolved)

	_, err = service.Confirm(ctx, userID, unmatched.ID)
	require.ErrorIs(t, err, ErrInvalidCandidate)
	ignored, err := service.Ignore(ctx, userID, unmatched.ID)
	require.NoError(t, err)
	replayedIgnored, err := service.Ignore(ctx, userID, unmatched.ID)
	require.NoError(t, err)
	require.Equal(t, ignored.ID, replayedIgnored.ID)
	_, err = service.Rematch(ctx, "other-user", unmatched.ID, "confirm-target", "")
	require.ErrorIs(t, err, ErrCandidateNotFound)
	_, err = service.Candidates(ctx, "", "")
	require.ErrorIs(t, err, ErrInvalidCandidate)
	_, err = service.Candidates(ctx, userID, CandidateStatus("unknown"))
	require.ErrorIs(t, err, ErrInvalidCandidate)
	_, err = service.Status(ctx, "")
	require.ErrorIs(t, err, ErrInvalidCandidate)

	customEvent := candidateMovieEvent("event-custom-invalid", "provider-custom-invalid", "Custom", 2026)
	customCandidate, err := service.Ingest(ctx, accountID, customEvent)
	require.NoError(t, err)
	for _, input := range []CustomMediaInput{
		{Title: "", MediaType: "movie", Year: "2026"},
		{Title: "Custom", MediaType: "tv", Year: "2026"},
		{Title: "Custom", MediaType: "movie", Year: "1800"},
	} {
		_, err := service.CreateCustom(ctx, userID, customCandidate.ID, input)
		require.ErrorIs(t, err, ErrInvalidCandidate)
	}
}

func TestConfirmedMappingReusesEvidenceAndProjectsSyncState(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "mapped-media", "movie", "Mapped Film", "2025")
	firstEvent := candidateMovieEvent("mapped-event-1", "mapped-provider-item", "Mapped Film", 2025)
	firstEvent.PositionSeconds = firstEvent.DurationSeconds / 2
	firstCandidate, err := service.Ingest(ctx, accountID, firstEvent)
	require.NoError(t, err)
	require.Equal(t, CandidatePossible, firstCandidate.Status)
	_, err = service.Rematch(ctx, userID, firstCandidate.ID, "mapped-media", "")
	require.NoError(t, err)

	secondEvent := candidateMovieEvent("mapped-event-2", "mapped-provider-item", "Different Provider Title", 2025)
	secondCandidate, err := service.Ingest(ctx, accountID, secondEvent)
	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, secondCandidate.Status)
	require.Equal(t, "mapped-media", secondCandidate.MediaID)
	require.Equal(t, "confirmed_mapping", secondCandidate.Evidence[0].Code)
	var state string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT status FROM user_media_states WHERE user_id = ? AND media_id = 'mapped-media'
	`, userID).Scan(&state))
	require.Equal(t, "completed", state)

	thirdEvent := candidateMovieEvent("mapped-event-3", "mapped-provider-item", "Another Title", 2025)
	thirdCandidate, err := service.Ingest(ctx, accountID, thirdEvent)
	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, thirdCandidate.Status)
	require.Equal(t, 3, countRows(t, db, "watch_events"))
}

func TestCandidateValidationAndProjectionHelpers(t *testing.T) {
	for _, status := range []CandidateStatus{
		CandidateExact, CandidatePossible, CandidateUnmatched,
		CandidateConflict, CandidateConfirmed, CandidateIgnored,
	} {
		require.True(t, validCandidateStatus(status))
	}
	require.False(t, validCandidateStatus(""))
	require.True(t, validCandidateYear(""))
	require.True(t, validCandidateYear("2026"))
	require.False(t, validCandidateYear("26"))
	require.False(t, validCandidateYear("abcd"))
	require.False(t, validCandidateYear("2201"))

	require.Equal(t, 100, syncCompletion(integrations.HistoryEvent{}))
	require.Equal(t, 1, syncCompletion(integrations.HistoryEvent{DurationSeconds: 1000, PositionSeconds: 1}))
	require.Equal(t, 50, syncCompletion(integrations.HistoryEvent{DurationSeconds: 100, PositionSeconds: 50}))
	require.Equal(t, 100, syncCompletion(integrations.HistoryEvent{DurationSeconds: 100, PositionSeconds: 200}))
	require.Equal(t, "Jellyfin", providerDisplayName("jellyfin"))
	require.Equal(t, "Emby", providerDisplayName("emby"))
	require.Equal(t, "Plex", providerDisplayName("plex"))
	require.Equal(t, "媒体服务器", providerDisplayName("unknown"))
	require.Nil(t, nullableCandidateInt(0))
	require.Equal(t, 12, nullableCandidateInt(12))
	require.Nil(t, nullStringValue(sql.NullString{}))
	require.Equal(t, "2026-07-13", nullStringValue(sql.NullString{String: "2026-07-13", Valid: true}))
	require.NotNil(t, NewCandidateService(nil, CandidateServiceOptions{}).now)
	require.Equal(t, "sync provider run failed", (&runDueError{
		providerErrors: []error{errors.New("synthetic provider failure")},
	}).Error())
	require.Equal(t, "sync scheduler operation failed", (&runDueError{
		schedulerErrors: []error{errors.New("synthetic scheduler failure")},
	}).Error())
}

func TestCandidateRematchValidatesMediaAndEpisodeOwnership(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "target-movie", "movie", "Target Movie", "2026")
	insertCandidateMedia(t, db, "target-series-a", "tv", "Target Series A", "2026")
	insertCandidateMedia(t, db, "target-series-b", "tv", "Target Series B", "2026")
	insertCandidateEpisode(t, db, "target-series-a", "target-season-a", "target-episode-a")
	insertCandidateEpisode(t, db, "target-series-b", "target-season-b", "target-episode-b")
	movieCandidate, err := service.Ingest(ctx, accountID, candidateMovieEvent(
		"invalid-movie-target", "invalid-movie-provider", "No Movie Match", 2026,
	))
	require.NoError(t, err)
	episodeCandidate, err := service.Ingest(ctx, accountID, candidateEpisodeEvent(
		"invalid-episode-target", "invalid-episode-provider", "No Series Match", 2026, 1, 1,
	))
	require.NoError(t, err)

	_, err = service.Rematch(ctx, userID, movieCandidate.ID, "target-series-a", "")
	require.ErrorIs(t, err, ErrInvalidCandidate)
	_, err = service.Rematch(ctx, userID, episodeCandidate.ID, "target-movie", "target-episode-a")
	require.ErrorIs(t, err, ErrInvalidCandidate)
	_, err = service.Rematch(ctx, userID, episodeCandidate.ID, "target-series-a", "")
	require.ErrorIs(t, err, ErrInvalidCandidate)
	_, err = service.Rematch(ctx, userID, episodeCandidate.ID, "target-series-a", "target-episode-b")
	require.ErrorIs(t, err, ErrInvalidCandidate)
}

func TestApplyCandidateIsIdempotentAndRejectsExternalEventRetargeting(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "apply-media-a", "movie", "Apply A", "2026")
	insertCandidateMedia(t, db, "apply-media-b", "movie", "Apply B", "2026")
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	candidate := Candidate{
		ID: "apply-candidate", AccountID: accountID,
		Event: candidateMovieEvent("apply-event", "apply-provider-item", "Apply A", 2026),
	}

	tx, err := db.Writer().BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, service.applyCandidate(
		ctx, tx, userID, "jellyfin", candidate, "apply-media-a", "", now,
	))
	require.NoError(t, tx.Commit())
	tx, err = db.Writer().BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, service.applyCandidate(
		ctx, tx, userID, "jellyfin", candidate, "apply-media-a", "", now,
	))
	require.NoError(t, tx.Commit())
	require.Equal(t, 1, countRows(t, db, "watch_events"))

	tx, err = db.Writer().BeginTx(ctx, nil)
	require.NoError(t, err)
	require.ErrorIs(t, service.applyCandidate(
		ctx, tx, userID, "jellyfin", candidate, "apply-media-b", "", now,
	), ErrCandidateResolved)
	require.NoError(t, tx.Rollback())
}

func insertCandidateEpisode(t *testing.T, db *storage.DB, mediaID, seasonID, episodeID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO seasons (id, media_id, source_id, season_number, name, overview, poster_path, air_date)
		VALUES (?, ?, NULL, 1, 'Season 1', '', '', '')
	`, seasonID, mediaID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO episodes (
			id, season_id, source_id, episode_number, name, overview, still_path, air_date
		) VALUES (?, ?, NULL, 1, 'Episode 1', '', '', '')
	`, episodeID, seasonID)
	require.NoError(t, err)
}
