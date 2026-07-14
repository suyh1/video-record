package sync

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"video-record/internal/integrations"
	"video-record/internal/storage"
)

func TestMatchExactExternalIDAutoAppliesConflictFreeCandidate(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "movie-exact", "movie", "Exact Movie", "2026")
	insertCandidateExternalID(t, db, "movie-exact", "tmdb", "603", "movie")
	event := candidateMovieEvent("event-exact", "provider-movie", "Exact Movie", 2026)
	event.Item.TMDBID = "603"

	candidate, err := service.Ingest(ctx, accountID, event)

	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, candidate.Status)
	require.Equal(t, "movie-exact", candidate.MediaID)
	require.NotEmpty(t, candidate.Evidence)
	require.Contains(t, candidate.Evidence[0].Text, "TMDB")
	require.Equal(t, 1, countRows(t, db, "watch_events"))
	require.Equal(t, 1, countRows(t, db, "watch_event_participants"))
	require.Equal(t, 1, countRows(t, db, "external_media_mappings"))
	var status, statusSource, externalEventID string
	var stateUpdatedAt int64
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT round.status, round.status_source, round.updated_at, event.external_event_id
		FROM watch_rounds round
		JOIN watch_events event ON event.round_id = round.id
		WHERE round.user_id = ? AND round.media_id = ? AND round.archived_at IS NULL
	`, userID, "movie-exact").Scan(&status, &statusSource, &stateUpdatedAt, &externalEventID))
	require.Equal(t, "completed", status)
	require.Equal(t, "confirmed_sync", statusSource)
	require.Positive(t, stateUpdatedAt)
	require.Equal(t, accountID+":"+event.ID, externalEventID)
}

func TestMatchTitleYearAmbiguityAndMediaTypeMismatchRequireReview(t *testing.T) {
	ctx := context.Background()
	service, db, _, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "movie-a", "movie", "Shared Title", "2024")
	insertCandidateMedia(t, db, "movie-b", "movie", "Shared Title", "2024")

	ambiguous, err := service.Ingest(ctx, accountID, candidateMovieEvent(
		"event-ambiguous", "provider-ambiguous", "Shared Title", 2024,
	))
	require.NoError(t, err)
	require.Equal(t, CandidatePossible, ambiguous.Status)
	require.Len(t, ambiguous.Options, 2)
	require.Contains(t, ambiguous.Evidence[0].Text, "2 个")

	insertCandidateMedia(t, db, "series-mismatch", "tv", "Wrong Type", "2025")
	insertCandidateExternalID(t, db, "series-mismatch", "tmdb", "999", "tv")
	mismatchEvent := candidateMovieEvent("event-mismatch", "provider-mismatch", "Wrong Type", 2025)
	mismatchEvent.Item.TMDBID = "999"
	mismatch, err := service.Ingest(ctx, accountID, mismatchEvent)
	require.NoError(t, err)
	require.Equal(t, CandidateConflict, mismatch.Status)
	require.Contains(t, mismatch.Evidence[0].Text, "类型")
	require.Equal(t, 0, countRows(t, db, "watch_events"))
}

func TestConflictWhenExternalIdentityHintsDisagree(t *testing.T) {
	ctx := context.Background()
	service, db, _, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "identity-movie", "movie", "Identity Film", "2026")
	insertCandidateMedia(t, db, "identity-series", "tv", "Identity Series", "2026")
	insertCandidateExternalID(t, db, "identity-movie", "tmdb", "1001", "movie")
	insertCandidateExternalID(t, db, "identity-series", "imdb", "tt1001", "tv")
	disagreeing := candidateMovieEvent("event-disagreeing-type", "provider-disagreeing", "Identity Film", 2026)
	disagreeing.Item.TMDBID = "1001"
	disagreeing.Item.IMDbID = "tt1001"

	typeConflict, err := service.Ingest(ctx, accountID, disagreeing)

	require.NoError(t, err)
	require.Equal(t, CandidateConflict, typeConflict.Status)
	require.Contains(t, typeConflict.Evidence[0].Text, "类型")
	require.Equal(t, 0, countRows(t, db, "watch_events"))

	insertCandidateMedia(t, db, "other-identity-movie", "movie", "Other Identity Film", "2026")
	insertCandidateExternalID(t, db, "other-identity-movie", "imdb", "tt1002", "movie")
	differentMovies := candidateMovieEvent("event-different-movies", "provider-different", "Identity Film", 2026)
	differentMovies.Item.TMDBID = "1001"
	differentMovies.Item.IMDbID = "tt1002"

	identityConflict, err := service.Ingest(ctx, accountID, differentMovies)

	require.NoError(t, err)
	require.Equal(t, CandidateConflict, identityConflict.Status)
	require.Len(t, identityConflict.Options, 2)
	require.Contains(t, identityConflict.Evidence[0].Text, "2 个")
}

func TestConflictWithManualStatePreventsAutomaticApply(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "manual-movie", "movie", "Manual Movie", "2023")
	insertCandidateExternalID(t, db, "manual-movie", "tmdb", "777", "movie")
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status, version,
			status_source, rating_source, note_source, created_at, updated_at
		) VALUES ('manual-movie-round', ?, 'manual-movie', NULL, 1, 'dropped', 1,
		          'manual', 'manual', 'manual', 0, 0)
	`, userID)
	require.NoError(t, err)
	event := candidateMovieEvent("event-manual", "provider-manual", "Manual Movie", 2023)
	event.Item.TMDBID = "777"

	candidate, err := service.Ingest(ctx, accountID, event)

	require.NoError(t, err)
	require.Equal(t, CandidateConflict, candidate.Status)
	require.Contains(t, candidate.Evidence[len(candidate.Evidence)-1].Text, "手工")
	require.Equal(t, 0, countRows(t, db, "watch_events"))
	var status string
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
		SELECT status FROM watch_rounds WHERE user_id = ? AND media_id = 'manual-movie'
	`, userID).Scan(&status))
	require.Equal(t, "dropped", status)
}

func TestConflictWithManualCompletedStatePreventsAutomaticApply(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "manual-completed", "movie", "Already Watched", "2023")
	insertCandidateExternalID(t, db, "manual-completed", "tmdb", "778", "movie")
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO watch_rounds (
			id, user_id, media_id, season_number, round_number, status, version,
			status_source, rating_source, note_source, created_at, updated_at
		) VALUES ('manual-completed-round', ?, 'manual-completed', NULL, 1, 'completed', 1,
		          'manual', 'manual', 'manual', 0, 0)
	`, userID)
	require.NoError(t, err)
	event := candidateMovieEvent("event-manual-completed", "provider-manual-completed", "Already Watched", 2023)
	event.Item.TMDBID = "778"

	candidate, err := service.Ingest(ctx, accountID, event)

	require.NoError(t, err)
	require.Equal(t, CandidateConflict, candidate.Status)
	require.Contains(t, candidate.Evidence[len(candidate.Evidence)-1].Text, "手工")
	require.Equal(t, 0, countRows(t, db, "watch_events"))
}

func TestMatchOriginalTitleAndYearRequiresReview(t *testing.T) {
	ctx := context.Background()
	service, db, _, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "original-title", "movie", "The Original Title", "2021")
	event := candidateMovieEvent("event-original-title", "provider-original-title", "本地化标题", 2021)
	event.Item.OriginalTitle = "The Original Title"

	candidate, err := service.Ingest(ctx, accountID, event)

	require.NoError(t, err)
	require.Equal(t, CandidatePossible, candidate.Status)
	require.Equal(t, "original-title", candidate.MediaID)
	require.Equal(t, "The Original Title", candidate.Options[0].OriginalTitle)
	require.Contains(t, candidate.Evidence[0].Text, "标题和年份")
}

func TestCandidateIgnoreRematchCustomEpisodeAndRepeatedExternalIDs(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "choice-a", "movie", "Choice", "2022")
	insertCandidateMedia(t, db, "choice-b", "movie", "Choice", "2022")
	choiceEvent := candidateMovieEvent("event-choice", "provider-choice", "Choice", 2022)
	choice, err := service.Ingest(ctx, accountID, choiceEvent)
	require.NoError(t, err)
	require.Equal(t, CandidatePossible, choice.Status)

	rematched, err := service.Rematch(ctx, userID, choice.ID, "choice-b", "")
	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, rematched.Status)
	require.Equal(t, "choice-b", rematched.MediaID)
	replayed, err := service.Ingest(ctx, accountID, choiceEvent)
	require.NoError(t, err)
	require.Equal(t, rematched.ID, replayed.ID)
	require.Equal(t, 1, countRows(t, db, "watch_events"))

	unmatchedEvent := candidateMovieEvent("event-ignore", "provider-ignore", "Never Match", 1998)
	unmatched, err := service.Ingest(ctx, accountID, unmatchedEvent)
	require.NoError(t, err)
	require.Equal(t, CandidateUnmatched, unmatched.Status)
	ignored, err := service.Ignore(ctx, userID, unmatched.ID)
	require.NoError(t, err)
	require.Equal(t, CandidateIgnored, ignored.Status)
	replayedIgnored, err := service.Ingest(ctx, accountID, unmatchedEvent)
	require.NoError(t, err)
	require.Equal(t, CandidateIgnored, replayedIgnored.Status)

	episodeEvent := candidateEpisodeEvent("event-custom", "provider-episode", "Custom Series", 2025, 2, 3)
	customCandidate, err := service.Ingest(ctx, accountID, episodeEvent)
	require.NoError(t, err)
	require.Equal(t, CandidateUnmatched, customCandidate.Status)
	custom, err := service.CreateCustom(ctx, userID, customCandidate.ID, CustomMediaInput{
		Title: "Custom Series", MediaType: "tv", Year: "2025",
	})
	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, custom.Status)
	require.NotEmpty(t, custom.MediaID)
	require.NotEmpty(t, custom.EpisodeID)
	var episodeProgress int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
			SELECT COUNT(*) FROM round_episode_progress progress
			JOIN watch_rounds round ON round.id = progress.round_id
			WHERE round.user_id = ? AND round.media_id = ? AND progress.episode_id = ?
	`, userID, custom.MediaID, custom.EpisodeID).Scan(&episodeProgress))
	require.Equal(t, 1, episodeProgress)
	require.Equal(t, 2, countRows(t, db, "watch_events"))
	require.Equal(t, 3, countRows(t, db, "audit_events"))
}

func TestMatchExactEpisodeByTMDBIDCreatesProgress(t *testing.T) {
	ctx := context.Background()
	service, db, userID, accountID := newCandidateService(t)
	insertCandidateMedia(t, db, "series-exact", "tv", "Exact Series", "2026")
	_, err := db.Writer().ExecContext(ctx, `
		INSERT INTO seasons (id, media_id, source_id, season_number, name, overview, poster_path, air_date)
		VALUES ('season-exact', 'series-exact', 'season-source', 1, 'Season 1', '', '', '')
	`)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO episodes (
			id, season_id, source_id, episode_number, name, overview, still_path, air_date
		) VALUES ('episode-exact', 'season-exact', '30007', 7, 'Episode 7', '', '', '')
	`)
	require.NoError(t, err)
	event := candidateEpisodeEvent("event-episode", "provider-episode-exact", "Exact Series", 2026, 1, 7)
	event.Item.TMDBID = "30007"

	candidate, err := service.Ingest(ctx, accountID, event)

	require.NoError(t, err)
	require.Equal(t, CandidateConfirmed, candidate.Status)
	require.Equal(t, "series-exact", candidate.MediaID)
	require.Equal(t, "episode-exact", candidate.EpisodeID)
	var progressCount int
	require.NoError(t, db.Reader().QueryRowContext(ctx, `
			SELECT COUNT(*) FROM round_episode_progress progress
			JOIN watch_rounds round ON round.id = progress.round_id
			WHERE round.user_id = ? AND progress.episode_id = 'episode-exact'
	`, userID).Scan(&progressCount))
	require.Equal(t, 1, progressCount)
}

func newCandidateService(t *testing.T) (*CandidateService, *storage.DB, string, string) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "video-record.db"))
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(ctx, db))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	userID, accountID := "sync-user", "sync-account"
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, active, created_at)
		VALUES (?, 'sync-owner', 'synthetic-hash', 'admin', 1, 0)
	`, userID)
	require.NoError(t, err)
	_, err = db.Writer().ExecContext(ctx, `
		INSERT INTO external_accounts (
			id, user_id, provider, name, base_url, credential_ciphertext,
			credential_nonce, credential_version, credential_fingerprint,
			enabled, created_at, updated_at
		) VALUES (?, ?, 'jellyfin', 'Home', 'https://media.example.test',
		          x'01', x'02', 1, 'fingerprint', 1, 0, 0)
	`, accountID, userID)
	require.NoError(t, err)
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	return NewCandidateService(db, CandidateServiceOptions{Now: func() time.Time { return now }}), db, userID, accountID
}

func insertCandidateMedia(t *testing.T, db *storage.DB, id, mediaType, title, year string) {
	t.Helper()
	_, err := db.Writer().ExecContext(context.Background(), `
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, custom_title, custom_year,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, '', '', '', NULL, NULL, 0, 0)
	`, id, mediaType, title, title, year+"-01-01")
	require.NoError(t, err)
}

func insertCandidateExternalID(t *testing.T, db *storage.DB, mediaID, source, sourceID, mediaType string) {
	t.Helper()
	_, err := db.Writer().ExecContext(context.Background(), `
		INSERT INTO media_external_ids (media_id, source, source_id, media_type)
		VALUES (?, ?, ?, ?)
	`, mediaID, source, sourceID, mediaType)
	require.NoError(t, err)
}

func candidateMovieEvent(id, providerItemID, title string, year int) integrations.HistoryEvent {
	return integrations.HistoryEvent{
		ID: id, PlayedAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		DurationSeconds: 7200, PositionSeconds: 7200,
		Item: integrations.ItemIdentity{
			ProviderItemID: providerItemID, MediaType: integrations.MediaMovie,
			Title: title, Year: year,
		},
	}
}

func candidateEpisodeEvent(
	id, providerItemID, title string,
	year, season, episode int,
) integrations.HistoryEvent {
	return integrations.HistoryEvent{
		ID: id, PlayedAt: time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC),
		DurationSeconds: 3600, PositionSeconds: 3600,
		Item: integrations.ItemIdentity{
			ProviderItemID: providerItemID, MediaType: integrations.MediaEpisode,
			Title: title, Year: year, SeasonNumber: season, EpisodeNumber: episode,
		},
	}
}

func countRows(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Reader().QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM "+table,
	).Scan(&count))
	return count
}
