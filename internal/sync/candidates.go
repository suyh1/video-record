package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"video-record/internal/integrations"
	"video-record/internal/records"
	"video-record/internal/storage"
)

const syncEventTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

var (
	ErrCandidateNotFound = errors.New("sync candidate not found")
	ErrCandidateResolved = errors.New("sync candidate is already resolved")
	ErrInvalidCandidate  = errors.New("invalid sync candidate")
	ErrSyncForbidden     = errors.New("sync candidate access forbidden")
)

type Candidate struct {
	ID              string                    `json:"id"`
	AccountID       string                    `json:"accountId"`
	ExternalEventID string                    `json:"externalEventId"`
	Status          CandidateStatus           `json:"status"`
	MediaID         string                    `json:"mediaId,omitempty"`
	EpisodeID       string                    `json:"episodeId,omitempty"`
	Event           integrations.HistoryEvent `json:"-"`
	Evidence        []MatchEvidence           `json:"evidence"`
	Options         []MatchOption             `json:"options"`
	CreatedAt       time.Time                 `json:"createdAt"`
	UpdatedAt       time.Time                 `json:"updatedAt"`
}

type CustomMediaInput struct {
	Title     string `json:"title"`
	MediaType string `json:"mediaType"`
	Year      string `json:"year"`
}

type CandidateServiceOptions struct {
	Now func() time.Time
}

type CandidateService struct {
	db      *storage.DB
	matcher *Matcher
	now     func() time.Time
}

type AccountSyncStatus struct {
	ID                string     `json:"id"`
	Provider          string     `json:"provider"`
	Name              string     `json:"name"`
	Enabled           bool       `json:"enabled"`
	PendingCandidates int        `json:"pendingCandidates"`
	LastRunStatus     string     `json:"lastRunStatus,omitempty"`
	LastRunAt         *time.Time `json:"lastRunAt,omitempty"`
}

type SyncStatus struct {
	Accounts     []AccountSyncStatus `json:"accounts"`
	PendingTotal int                 `json:"pendingTotal"`
}

type candidatePayload struct {
	Event    integrations.HistoryEvent `json:"event"`
	Evidence []MatchEvidence           `json:"evidence"`
	Options  []MatchOption             `json:"options"`
}

func NewCandidateService(db *storage.DB, options CandidateServiceOptions) *CandidateService {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &CandidateService{db: db, matcher: NewMatcher(db), now: now}
}

func (service *CandidateService) Ingest(
	ctx context.Context,
	accountID string,
	event integrations.HistoryEvent,
) (Candidate, error) {
	if existing, found, err := service.candidateByExternalEvent(ctx, accountID, event.ID); err != nil {
		return Candidate{}, err
	} else if found {
		return existing, nil
	}
	var userID, provider string
	err := service.db.Reader().QueryRowContext(ctx, `
		SELECT user_id, provider FROM external_accounts WHERE id = ? AND enabled = 1
	`, accountID).Scan(&userID, &provider)
	if errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, ErrCandidateNotFound
	}
	if err != nil {
		return Candidate{}, err
	}
	result, err := service.matcher.Match(ctx, accountID, userID, event)
	if err != nil {
		return Candidate{}, err
	}
	payload, err := json.Marshal(candidatePayload{
		Event: event, Evidence: result.Evidence, Options: result.Options,
	})
	if err != nil {
		return Candidate{}, err
	}
	now := service.now().UTC()
	candidate := Candidate{
		ID: uuid.NewString(), AccountID: accountID, ExternalEventID: event.ID,
		Status: result.Status, MediaID: result.MediaID, EpisodeID: result.EpisodeID,
		Event: event, Evidence: result.Evidence, Options: result.Options,
		CreatedAt: now, UpdatedAt: now,
	}
	tx, err := service.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return Candidate{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if result.Status == CandidateExact {
		if err := service.applyCandidate(
			ctx, tx, userID, provider, candidate, result.MediaID, result.EpisodeID, now,
		); err != nil {
			return Candidate{}, err
		}
		candidate.Status = CandidateConfirmed
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO sync_candidates (
			id, account_id, external_event_id, status, payload_json,
			media_id, episode_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, candidate.ID, accountID, event.ID, candidate.Status, string(payload),
		nullableCandidateString(candidate.MediaID), nullableCandidateString(candidate.EpisodeID),
		now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

func (service *CandidateService) Candidates(
	ctx context.Context,
	userID string,
	status CandidateStatus,
) ([]Candidate, error) {
	if userID == "" || status != "" && !validCandidateStatus(status) {
		return nil, ErrInvalidCandidate
	}
	query := `
		SELECT candidate.id, candidate.account_id, candidate.external_event_id,
		       candidate.status, candidate.payload_json, candidate.media_id, candidate.episode_id,
		       candidate.created_at, candidate.updated_at
		FROM sync_candidates candidate
		JOIN external_accounts account ON account.id = candidate.account_id
		WHERE account.user_id = ?`
	arguments := []any{userID}
	if status != "" {
		query += " AND candidate.status = ?"
		arguments = append(arguments, status)
	}
	query += " ORDER BY candidate.updated_at DESC, candidate.id"
	rows, err := service.db.Reader().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]Candidate, 0)
	for rows.Next() {
		candidate, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (service *CandidateService) Status(ctx context.Context, userID string) (SyncStatus, error) {
	if userID == "" {
		return SyncStatus{}, ErrInvalidCandidate
	}
	rows, err := service.db.Reader().QueryContext(ctx, `
		SELECT account.id, account.provider, account.name, account.enabled,
		       COUNT(CASE WHEN candidate.status IN ('exact', 'possible', 'unmatched', 'conflict') THEN 1 END),
		       COALESCE((
		           SELECT run.status FROM sync_runs run
		           WHERE run.account_id = account.id
		           ORDER BY run.started_at DESC, run.id DESC LIMIT 1
		       ), ''),
		       (
		           SELECT COALESCE(run.finished_at, run.started_at) FROM sync_runs run
		           WHERE run.account_id = account.id
		           ORDER BY run.started_at DESC, run.id DESC LIMIT 1
		       )
		FROM external_accounts account
		LEFT JOIN sync_candidates candidate ON candidate.account_id = account.id
		WHERE account.user_id = ?
		GROUP BY account.id
		ORDER BY account.provider, account.name, account.id
	`, userID)
	if err != nil {
		return SyncStatus{}, err
	}
	defer func() { _ = rows.Close() }()
	status := SyncStatus{Accounts: make([]AccountSyncStatus, 0)}
	for rows.Next() {
		var account AccountSyncStatus
		var lastRunAt sql.NullInt64
		if err := rows.Scan(
			&account.ID, &account.Provider, &account.Name, &account.Enabled,
			&account.PendingCandidates, &account.LastRunStatus, &lastRunAt,
		); err != nil {
			return SyncStatus{}, err
		}
		if lastRunAt.Valid {
			value := time.UnixMilli(lastRunAt.Int64).UTC()
			account.LastRunAt = &value
		}
		status.PendingTotal += account.PendingCandidates
		status.Accounts = append(status.Accounts, account)
	}
	return status, rows.Err()
}

func (service *CandidateService) Confirm(ctx context.Context, userID, candidateID string) (Candidate, error) {
	candidate, err := service.candidateForUser(ctx, userID, candidateID)
	if err != nil {
		return Candidate{}, err
	}
	if candidate.MediaID == "" {
		return Candidate{}, ErrInvalidCandidate
	}
	return service.resolve(ctx, userID, candidate, candidate.MediaID, candidate.EpisodeID, "sync.candidate.confirm")
}

func (service *CandidateService) Rematch(
	ctx context.Context,
	userID, candidateID, mediaID, episodeID string,
) (Candidate, error) {
	candidate, err := service.candidateForUser(ctx, userID, candidateID)
	if err != nil {
		return Candidate{}, err
	}
	return service.resolve(ctx, userID, candidate, mediaID, episodeID, "sync.candidate.rematch")
}

func (service *CandidateService) Ignore(ctx context.Context, userID, candidateID string) (Candidate, error) {
	candidate, err := service.candidateForUser(ctx, userID, candidateID)
	if err != nil {
		return Candidate{}, err
	}
	if candidate.Status == CandidateIgnored {
		return candidate, nil
	}
	if candidate.Status == CandidateConfirmed {
		return Candidate{}, ErrCandidateResolved
	}
	now := service.now().UTC()
	tx, err := service.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return Candidate{}, err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := tx.ExecContext(ctx, `
		UPDATE sync_candidates SET status = 'ignored', updated_at = ?
		WHERE id = ? AND account_id IN (SELECT id FROM external_accounts WHERE user_id = ?)
		  AND status != 'confirmed'
	`, now.UnixMilli(), candidateID, userID)
	if err != nil {
		return Candidate{}, err
	}
	count, err := updated.RowsAffected()
	if err != nil || count != 1 {
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{}, ErrCandidateNotFound
	}
	if err := insertSyncAudit(ctx, tx, userID, "sync.candidate.ignore", candidateID, "", now); err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(); err != nil {
		return Candidate{}, err
	}
	candidate.Status, candidate.UpdatedAt = CandidateIgnored, now
	return candidate, nil
}

func (service *CandidateService) CreateCustom(
	ctx context.Context,
	userID, candidateID string,
	input CustomMediaInput,
) (Candidate, error) {
	candidate, err := service.candidateForUser(ctx, userID, candidateID)
	if err != nil {
		return Candidate{}, err
	}
	input.Title, input.Year = strings.TrimSpace(input.Title), strings.TrimSpace(input.Year)
	expectedType := "movie"
	if candidate.Event.Item.MediaType == integrations.MediaEpisode {
		expectedType = "tv"
	}
	if input.Title == "" || input.MediaType != expectedType || !validCandidateYear(input.Year) ||
		candidate.Status == CandidateConfirmed || candidate.Status == CandidateIgnored {
		return Candidate{}, ErrInvalidCandidate
	}
	now := service.now().UTC()
	tx, err := service.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return Candidate{}, err
	}
	defer func() { _ = tx.Rollback() }()
	mediaID := uuid.NewString()
	runtimeMinutes := candidate.Event.DurationSeconds / 60
	_, err = tx.ExecContext(ctx, `
		INSERT INTO media_items (
			id, media_type, external_title, original_title, release_date,
			external_overview, poster_path, backdrop_path, custom_title, custom_year,
			runtime_minutes, created_at, updated_at
		) VALUES (?, ?, '', '', '', '', '', '', ?, ?, ?, ?, ?)
	`, mediaID, input.MediaType, input.Title, nullableCandidateString(input.Year),
		nullableCandidateInt(runtimeMinutes), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Candidate{}, err
	}
	episodeID := ""
	if candidate.Event.Item.MediaType == integrations.MediaEpisode {
		seasonID := uuid.NewString()
		episodeID = uuid.NewString()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO seasons (id, media_id, source_id, season_number, name, overview, poster_path, air_date)
			VALUES (?, ?, NULL, ?, ?, '', '', '')
		`, seasonID, mediaID, candidate.Event.Item.SeasonNumber,
			fmt.Sprintf("第 %d 季", candidate.Event.Item.SeasonNumber))
		if err != nil {
			return Candidate{}, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO episodes (
				id, season_id, source_id, episode_number, name, overview, still_path, air_date, runtime
			) VALUES (?, ?, NULL, ?, ?, '', '', '', ?)
		`, episodeID, seasonID, candidate.Event.Item.EpisodeNumber,
			fmt.Sprintf("第 %d 集", candidate.Event.Item.EpisodeNumber), nullableCandidateInt(runtimeMinutes))
		if err != nil {
			return Candidate{}, err
		}
	}
	var provider string
	if err := tx.QueryRowContext(ctx, `
		SELECT provider FROM external_accounts WHERE id = ? AND user_id = ?
	`, candidate.AccountID, userID).Scan(&provider); err != nil {
		return Candidate{}, err
	}
	if err := service.applyCandidate(ctx, tx, userID, provider, candidate, mediaID, episodeID, now); err != nil {
		return Candidate{}, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sync_candidates SET status = 'confirmed', media_id = ?, episode_id = ?, updated_at = ?
		WHERE id = ?
	`, mediaID, nullableCandidateString(episodeID), now.UnixMilli(), candidate.ID)
	if err != nil {
		return Candidate{}, err
	}
	if err := insertSyncAudit(ctx, tx, userID, "sync.candidate.custom", candidate.ID, mediaID, now); err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(); err != nil {
		return Candidate{}, err
	}
	candidate.Status, candidate.MediaID, candidate.EpisodeID = CandidateConfirmed, mediaID, episodeID
	candidate.UpdatedAt = now
	return candidate, nil
}

func (service *CandidateService) resolve(
	ctx context.Context,
	userID string,
	candidate Candidate,
	mediaID, episodeID, action string,
) (Candidate, error) {
	if mediaID == "" {
		return Candidate{}, ErrInvalidCandidate
	}
	if candidate.Status == CandidateConfirmed {
		if candidate.MediaID == mediaID && candidate.EpisodeID == episodeID {
			return candidate, nil
		}
		return Candidate{}, ErrCandidateResolved
	}
	if candidate.Status == CandidateIgnored {
		return Candidate{}, ErrCandidateResolved
	}
	now := service.now().UTC()
	tx, err := service.db.Writer().BeginTx(ctx, nil)
	if err != nil {
		return Candidate{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateCandidateTarget(ctx, tx, candidate.Event.Item.MediaType, mediaID, episodeID); err != nil {
		return Candidate{}, err
	}
	var provider string
	if err := tx.QueryRowContext(ctx, `
		SELECT provider FROM external_accounts WHERE id = ? AND user_id = ?
	`, candidate.AccountID, userID).Scan(&provider); err != nil {
		return Candidate{}, ErrSyncForbidden
	}
	if err := service.applyCandidate(ctx, tx, userID, provider, candidate, mediaID, episodeID, now); err != nil {
		return Candidate{}, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sync_candidates SET status = 'confirmed', media_id = ?, episode_id = ?, updated_at = ?
		WHERE id = ?
	`, mediaID, nullableCandidateString(episodeID), now.UnixMilli(), candidate.ID)
	if err != nil {
		return Candidate{}, err
	}
	if err := insertSyncAudit(ctx, tx, userID, action, candidate.ID, mediaID, now); err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(); err != nil {
		return Candidate{}, err
	}
	candidate.Status, candidate.MediaID, candidate.EpisodeID = CandidateConfirmed, mediaID, episodeID
	candidate.UpdatedAt = now
	return candidate, nil
}

func (service *CandidateService) applyCandidate(
	ctx context.Context,
	tx *sql.Tx,
	userID, provider string,
	candidate Candidate,
	mediaID, episodeID string,
	now time.Time,
) error {
	if err := validateCandidateTarget(ctx, tx, candidate.Event.Item.MediaType, mediaID, episodeID); err != nil {
		return err
	}
	externalEventID := candidate.AccountID + ":" + candidate.Event.ID
	var existingMediaID string
	var existingEpisodeID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT media_id, episode_id FROM watch_events
		WHERE created_by_user_id = ? AND source = 'confirmed_sync' AND external_event_id = ?
	`, userID, externalEventID).Scan(&existingMediaID, &existingEpisodeID)
	if err == nil {
		if existingMediaID != mediaID || existingEpisodeID.String != episodeID {
			return ErrCandidateResolved
		}
		return upsertExternalMapping(ctx, tx, candidate, mediaID, episodeID, now)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	completion := syncCompletion(candidate.Event)
	if _, err := records.ApplySyncedWatch(ctx, tx, records.SyncedWatchInput{
		UserID: userID, MediaID: mediaID, EpisodeID: episodeID,
		WatchedAt: candidate.Event.PlayedAt, ViewingMethod: providerDisplayName(provider),
		ExternalEventID: externalEventID, Completion: completion, Now: now,
	}); err != nil {
		return err
	}
	return upsertExternalMapping(ctx, tx, candidate, mediaID, episodeID, now)
}

func ensureSyncedState(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID string,
	episode bool,
	completion int,
	now time.Time,
) error {
	desired := records.StatusWatching
	if completion >= 90 {
		desired = records.StatusCompleted
	}
	if episode && completion >= 90 {
		var watched, total int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(progress.episode_id), COUNT(episode.id)
			FROM episodes episode
			JOIN seasons season ON season.id = episode.season_id
			LEFT JOIN episode_progress progress
			  ON progress.episode_id = episode.id AND progress.user_id = ?
			WHERE season.media_id = ?
		`, userID, mediaID).Scan(&watched, &total); err != nil {
			return err
		}
		if total == 0 || watched < total {
			desired = records.StatusWatching
		}
	}
	var current records.Status
	var source records.Source
	var version int
	err := tx.QueryRowContext(ctx, `
		SELECT status, status_source, version FROM user_media_states
		WHERE user_id = ? AND media_id = ?
	`, userID, mediaID).Scan(&current, &source, &version)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_media_states (
				user_id, media_id, status, version,
				status_source, rating_source, note_source, updated_at
			) VALUES (?, ?, ?, 1, 'confirmed_sync', 'confirmed_sync', 'confirmed_sync', ?)
		`, userID, mediaID, desired, now.UnixMilli())
		return err
	}
	if err != nil {
		return err
	}
	if current == records.StatusDropped || !records.CanOverwrite(records.SourceConfirmedSync, source) {
		return nil
	}
	if current == desired && source == records.SourceConfirmedSync {
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE user_media_states SET status = ?, status_source = 'confirmed_sync',
		       version = ?, updated_at = ?
		WHERE user_id = ? AND media_id = ? AND version = ?
	`, desired, version+1, now.UnixMilli(), userID, mediaID, version)
	return err
}

func recomputeSyncedDates(
	ctx context.Context,
	tx *sql.Tx,
	userID, mediaID string,
	now time.Time,
) error {
	var startedAt, completedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT MIN(event.watched_at), MAX(event.watched_at)
		FROM watch_events event
		JOIN watch_event_participants participant ON participant.event_id = event.id
		WHERE participant.user_id = ? AND event.media_id = ?
	`, userID, mediaID).Scan(&startedAt, &completedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE user_media_states SET started_at = ?, completed_at = ?, updated_at = ?
		WHERE user_id = ? AND media_id = ?
	`, nullStringValue(startedAt), nullStringValue(completedAt), now.UnixMilli(), userID, mediaID)
	return err
}

func upsertExternalMapping(
	ctx context.Context,
	tx *sql.Tx,
	candidate Candidate,
	mediaID, episodeID string,
	now time.Time,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO external_media_mappings (
			account_id, provider_item_id, media_type, media_id, episode_id, confirmed, updated_at
		) VALUES (?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(account_id, provider_item_id, media_type) DO UPDATE SET
			media_id = excluded.media_id, episode_id = excluded.episode_id,
			confirmed = 1, updated_at = excluded.updated_at
	`, candidate.AccountID, candidate.Event.Item.ProviderItemID, candidate.Event.Item.MediaType,
		mediaID, nullableCandidateString(episodeID), now.UnixMilli())
	return err
}

func validateCandidateTarget(
	ctx context.Context,
	tx *sql.Tx,
	eventType integrations.MediaType,
	mediaID, episodeID string,
) error {
	var mediaType string
	if err := tx.QueryRowContext(ctx, "SELECT media_type FROM media_items WHERE id = ?", mediaID).Scan(&mediaType); err != nil {
		return ErrInvalidCandidate
	}
	if eventType == integrations.MediaMovie {
		if mediaType != "movie" || episodeID != "" {
			return ErrInvalidCandidate
		}
		return nil
	}
	if eventType != integrations.MediaEpisode || mediaType != "tv" || episodeID == "" {
		return ErrInvalidCandidate
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		WHERE episode.id = ? AND season.media_id = ?
	`, episodeID, mediaID).Scan(&exists); err != nil || exists != 1 {
		return ErrInvalidCandidate
	}
	return nil
}

func insertSyncAudit(
	ctx context.Context,
	tx *sql.Tx,
	userID, action, candidateID, mediaID string,
	now time.Time,
) error {
	metadata, err := json.Marshal(map[string]string{"mediaId": mediaID})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, actor_user_id, action, target_type, target_id, metadata_json, created_at
		) VALUES (?, ?, ?, 'sync_candidate', ?, ?, ?)
	`, uuid.NewString(), userID, action, candidateID, string(metadata), now.UnixMilli())
	return err
}

func (service *CandidateService) candidateByExternalEvent(
	ctx context.Context,
	accountID, externalEventID string,
) (Candidate, bool, error) {
	row := service.db.Reader().QueryRowContext(ctx, `
		SELECT id, account_id, external_event_id, status, payload_json,
		       media_id, episode_id, created_at, updated_at
		FROM sync_candidates WHERE account_id = ? AND external_event_id = ?
	`, accountID, externalEventID)
	candidate, err := scanCandidate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, false, nil
	}
	return candidate, err == nil, err
}

func (service *CandidateService) candidateForUser(
	ctx context.Context,
	userID, candidateID string,
) (Candidate, error) {
	row := service.db.Reader().QueryRowContext(ctx, `
		SELECT candidate.id, candidate.account_id, candidate.external_event_id,
		       candidate.status, candidate.payload_json, candidate.media_id, candidate.episode_id,
		       candidate.created_at, candidate.updated_at
		FROM sync_candidates candidate
		JOIN external_accounts account ON account.id = candidate.account_id
		WHERE candidate.id = ? AND account.user_id = ?
	`, candidateID, userID)
	candidate, err := scanCandidate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Candidate{}, ErrCandidateNotFound
	}
	return candidate, err
}

type candidateScanner interface {
	Scan(...any) error
}

func scanCandidate(row candidateScanner) (Candidate, error) {
	var candidate Candidate
	var status string
	var payloadJSON string
	var mediaID, episodeID sql.NullString
	var createdAt, updatedAt int64
	if err := row.Scan(
		&candidate.ID, &candidate.AccountID, &candidate.ExternalEventID,
		&status, &payloadJSON, &mediaID, &episodeID, &createdAt, &updatedAt,
	); err != nil {
		return Candidate{}, err
	}
	var payload candidatePayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return Candidate{}, err
	}
	candidate.Status = CandidateStatus(status)
	candidate.MediaID, candidate.EpisodeID = mediaID.String, episodeID.String
	candidate.Event, candidate.Evidence, candidate.Options = payload.Event, payload.Evidence, payload.Options
	candidate.CreatedAt = time.UnixMilli(createdAt).UTC()
	candidate.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return candidate, nil
}

func validCandidateStatus(status CandidateStatus) bool {
	switch status {
	case CandidateExact, CandidatePossible, CandidateUnmatched, CandidateConflict, CandidateConfirmed, CandidateIgnored:
		return true
	default:
		return false
	}
}

func validCandidateYear(year string) bool {
	if year == "" {
		return true
	}
	if len(year) != 4 {
		return false
	}
	value, err := strconv.Atoi(year)
	return err == nil && value >= 1870 && value <= 2200
}

func syncCompletion(event integrations.HistoryEvent) int {
	if event.DurationSeconds <= 0 || event.PositionSeconds <= 0 {
		return 100
	}
	completion := event.PositionSeconds * 100 / event.DurationSeconds
	if completion < 1 {
		return 1
	}
	if completion > 100 {
		return 100
	}
	return completion
}

func providerDisplayName(provider string) string {
	switch provider {
	case "jellyfin":
		return "Jellyfin"
	case "emby":
		return "Emby"
	case "plex":
		return "Plex"
	default:
		return "媒体服务器"
	}
}

func nullableCandidateString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableCandidateInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullStringValue(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
