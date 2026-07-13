package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"video-record/internal/integrations"
	"video-record/internal/records"
	"video-record/internal/storage"
)

type CandidateStatus string

const (
	CandidateExact     CandidateStatus = "exact"
	CandidatePossible  CandidateStatus = "possible"
	CandidateUnmatched CandidateStatus = "unmatched"
	CandidateConflict  CandidateStatus = "conflict"
	CandidateConfirmed CandidateStatus = "confirmed"
	CandidateIgnored   CandidateStatus = "ignored"
)

type MatchEvidence struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

type MatchOption struct {
	MediaID       string `json:"mediaId"`
	EpisodeID     string `json:"episodeId,omitempty"`
	MediaType     string `json:"mediaType"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	Year          string `json:"year,omitempty"`
}

type MatchResult struct {
	Status    CandidateStatus `json:"status"`
	MediaID   string          `json:"mediaId,omitempty"`
	EpisodeID string          `json:"episodeId,omitempty"`
	Evidence  []MatchEvidence `json:"evidence"`
	Options   []MatchOption   `json:"options"`
}

type Matcher struct {
	db *storage.DB
}

func NewMatcher(db *storage.DB) *Matcher {
	return &Matcher{db: db}
}

func (matcher *Matcher) Match(
	ctx context.Context,
	accountID, userID string,
	event integrations.HistoryEvent,
) (MatchResult, error) {
	if accountID == "" || userID == "" || integrations.ValidateHistoryPage(
		integrations.HistoryPage{Events: []integrations.HistoryEvent{event}},
	) != nil {
		return MatchResult{}, ErrInvalidCandidate
	}
	if mapped, found, err := matcher.confirmedMapping(ctx, accountID, event.Item); err != nil {
		return MatchResult{}, err
	} else if found {
		result := MatchResult{
			Status: CandidateExact, MediaID: mapped.MediaID, EpisodeID: mapped.EpisodeID,
			Options: []MatchOption{mapped},
			Evidence: []MatchEvidence{{
				Code: "confirmed_mapping", Text: "已使用此前确认的媒体服务器映射",
			}},
		}
		return matcher.withUserConflict(ctx, userID, result)
	}

	exact, mismatch, evidence, err := matcher.externalMatches(ctx, event.Item)
	if err != nil {
		return MatchResult{}, err
	}
	if mismatch {
		return MatchResult{
			Status: CandidateConflict, Evidence: []MatchEvidence{{
				Code: "media_type_mismatch", Text: "外部 ID 已属于不同媒体类型，不能自动匹配",
			}}, Options: exact,
		}, nil
	}
	if len(exact) > 1 {
		return MatchResult{
			Status: CandidateConflict, Options: exact,
			Evidence: []MatchEvidence{{
				Code: "external_id_conflict", Text: fmt.Sprintf("外部 ID 指向 %d 个不同条目", len(exact)),
			}},
		}, nil
	}
	if len(exact) == 1 {
		result := MatchResult{
			Status: CandidateExact, MediaID: exact[0].MediaID, EpisodeID: exact[0].EpisodeID,
			Options: exact, Evidence: evidence,
		}
		return matcher.withUserConflict(ctx, userID, result)
	}

	options, err := matcher.titleMatches(ctx, event.Item)
	if err != nil {
		return MatchResult{}, err
	}
	if len(options) == 0 {
		return MatchResult{
			Status: CandidateUnmatched,
			Evidence: []MatchEvidence{{
				Code: "no_match", Text: "未找到可用的外部 ID、标题和年份匹配",
			}}, Options: []MatchOption{},
		}, nil
	}
	result := MatchResult{
		Status: CandidatePossible, Options: options,
		Evidence: []MatchEvidence{{
			Code: "title_year_match",
			Text: fmt.Sprintf("标题和年份找到 %d 个可能匹配，需要人工确认", len(options)),
		}},
	}
	if len(options) == 1 {
		result.MediaID, result.EpisodeID = options[0].MediaID, options[0].EpisodeID
	}
	return result, nil
}

func (matcher *Matcher) confirmedMapping(
	ctx context.Context,
	accountID string,
	identity integrations.ItemIdentity,
) (MatchOption, bool, error) {
	var mediaID string
	var episodeID sql.NullString
	err := matcher.db.Reader().QueryRowContext(ctx, `
		SELECT media_id, episode_id
		FROM external_media_mappings
		WHERE account_id = ? AND provider_item_id = ? AND media_type = ? AND confirmed = 1
	`, accountID, identity.ProviderItemID, identity.MediaType).Scan(&mediaID, &episodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return MatchOption{}, false, nil
	}
	if err != nil {
		return MatchOption{}, false, err
	}
	option, err := matcher.option(ctx, mediaID, episodeID.String)
	return option, err == nil, err
}

func (matcher *Matcher) externalMatches(
	ctx context.Context,
	identity integrations.ItemIdentity,
) ([]MatchOption, bool, []MatchEvidence, error) {
	if identity.MediaType == integrations.MediaEpisode {
		return matcher.episodeExternalMatches(ctx, identity)
	}
	wantedType := "movie"
	seen := map[string]struct{}{}
	options := make([]MatchOption, 0, 1)
	evidence := make([]MatchEvidence, 0, 3)
	mismatch := false
	identities := []struct {
		source, id, label string
	}{
		{"tmdb", identity.TMDBID, "TMDB"},
		{"imdb", identity.IMDbID, "IMDb"},
		{"tvdb", identity.TVDBID, "TVDB"},
	}
	for _, external := range identities {
		if external.id == "" {
			continue
		}
		rows, err := matcher.db.Reader().QueryContext(ctx, `
			SELECT media.id, media.media_type,
			       COALESCE(media.custom_title, media.external_title), media.original_title,
			       SUBSTR(COALESCE(NULLIF(media.release_date, ''), media.custom_year, ''), 1, 4)
			FROM media_external_ids external
			JOIN media_items media ON media.id = external.media_id
			WHERE external.source = ? AND external.source_id = ?
		`, external.source, external.id)
		if err != nil {
			return nil, false, nil, err
		}
		for rows.Next() {
			var option MatchOption
			if err := rows.Scan(
				&option.MediaID, &option.MediaType, &option.Title, &option.OriginalTitle, &option.Year,
			); err != nil {
				_ = rows.Close()
				return nil, false, nil, err
			}
			if option.MediaType != wantedType {
				mismatch = true
				continue
			}
			if _, exists := seen[option.MediaID]; exists {
				continue
			}
			seen[option.MediaID] = struct{}{}
			options = append(options, option)
			evidence = append(evidence, MatchEvidence{
				Code: external.source + "_id", Text: external.label + " ID 精确匹配",
			})
		}
		if err := rows.Close(); err != nil {
			return nil, false, nil, err
		}
	}
	return options, mismatch, evidence, nil
}

func (matcher *Matcher) episodeExternalMatches(
	ctx context.Context,
	identity integrations.ItemIdentity,
) ([]MatchOption, bool, []MatchEvidence, error) {
	if identity.TMDBID == "" {
		return nil, false, nil, nil
	}
	rows, err := matcher.db.Reader().QueryContext(ctx, `
		SELECT media.id, episode.id, media.media_type,
		       COALESCE(media.custom_title, media.external_title), media.original_title,
		       SUBSTR(COALESCE(NULLIF(media.release_date, ''), media.custom_year, ''), 1, 4)
		FROM episodes episode
		JOIN seasons season ON season.id = episode.season_id
		JOIN media_items media ON media.id = season.media_id
		WHERE episode.source_id = ?
	`, identity.TMDBID)
	if err != nil {
		return nil, false, nil, err
	}
	defer func() { _ = rows.Close() }()
	options := make([]MatchOption, 0, 1)
	for rows.Next() {
		var option MatchOption
		if err := rows.Scan(
			&option.MediaID, &option.EpisodeID, &option.MediaType,
			&option.Title, &option.OriginalTitle, &option.Year,
		); err != nil {
			return nil, false, nil, err
		}
		if option.MediaType != "tv" {
			return nil, true, nil, nil
		}
		options = append(options, option)
	}
	return options, false, []MatchEvidence{{Code: "tmdb_episode_id", Text: "TMDB 单集 ID 精确匹配"}}, rows.Err()
}

func (matcher *Matcher) titleMatches(
	ctx context.Context,
	identity integrations.ItemIdentity,
) ([]MatchOption, error) {
	mediaType := "movie"
	if identity.MediaType == integrations.MediaEpisode {
		mediaType = "tv"
	}
	year := ""
	if identity.Year > 0 {
		year = strconv.Itoa(identity.Year)
	}
	title := strings.TrimSpace(identity.Title)
	originalTitle := strings.TrimSpace(identity.OriginalTitle)
	rows, err := matcher.db.Reader().QueryContext(ctx, `
		SELECT id, media_type, COALESCE(custom_title, external_title), original_title,
		       SUBSTR(COALESCE(NULLIF(release_date, ''), custom_year, ''), 1, 4)
		FROM media_items
		WHERE media_type = ?
		  AND (COALESCE(custom_title, external_title) = ? COLLATE NOCASE
		       OR original_title = ? COLLATE NOCASE
		       OR (? != '' AND (COALESCE(custom_title, external_title) = ? COLLATE NOCASE
		                        OR original_title = ? COLLATE NOCASE)))
		  AND (? = '' OR SUBSTR(COALESCE(NULLIF(release_date, ''), custom_year, ''), 1, 4) = ?)
		ORDER BY id
		LIMIT 20
	`, mediaType, title, title, originalTitle, originalTitle, originalTitle, year, year)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	options := make([]MatchOption, 0)
	for rows.Next() {
		var option MatchOption
		if err := rows.Scan(
			&option.MediaID, &option.MediaType, &option.Title, &option.OriginalTitle, &option.Year,
		); err != nil {
			return nil, err
		}
		if identity.MediaType == integrations.MediaEpisode {
			_ = matcher.db.Reader().QueryRowContext(ctx, `
				SELECT episode.id
				FROM episodes episode
				JOIN seasons season ON season.id = episode.season_id
				WHERE season.media_id = ? AND season.season_number = ? AND episode.episode_number = ?
			`, option.MediaID, identity.SeasonNumber, identity.EpisodeNumber).Scan(&option.EpisodeID)
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func (matcher *Matcher) withUserConflict(
	ctx context.Context,
	userID string,
	result MatchResult,
) (MatchResult, error) {
	var status string
	var source records.Source
	err := matcher.db.Reader().QueryRowContext(ctx, `
		SELECT status, status_source FROM user_media_states WHERE user_id = ? AND media_id = ?
	`, userID, result.MediaID).Scan(&status, &source)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MatchResult{}, err
	}
	if err == nil && !records.CanOverwrite(records.SourceConfirmedSync, source) {
		result.Status = CandidateConflict
		label := "高优先级"
		if source == records.SourceManual {
			label = "手工"
		}
		result.Evidence = append(result.Evidence, MatchEvidence{
			Code: "personal_state_conflict", Text: label + "个人状态会阻止同步自动落档",
		})
	}
	return result, nil
}

func (matcher *Matcher) option(ctx context.Context, mediaID, episodeID string) (MatchOption, error) {
	var option MatchOption
	option.MediaID, option.EpisodeID = mediaID, episodeID
	err := matcher.db.Reader().QueryRowContext(ctx, `
		SELECT media_type, COALESCE(custom_title, external_title), original_title,
		       SUBSTR(COALESCE(NULLIF(release_date, ''), custom_year, ''), 1, 4)
		FROM media_items WHERE id = ?
	`, mediaID).Scan(&option.MediaType, &option.Title, &option.OriginalTitle, &option.Year)
	return option, err
}
