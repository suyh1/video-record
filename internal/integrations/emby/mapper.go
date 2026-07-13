package emby

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"video-record/internal/integrations"
)

const ticksPerSecond = 10_000_000

type userResponse struct {
	ID string `json:"Id"`
}

type historyResponse struct {
	UserID   string        `json:"user_id"`
	Activity []activityRow `json:"activity"`
}

type activityRow struct {
	PlayedAt     string `json:"Time"`
	ItemID       string `json:"Id"`
	Type         string `json:"Type"`
	Duration     string `json:"Duration"`
	RowID        string `json:"RowId"`
	numericRowID int64
	historyDate  time.Time
}

type itemResponse struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	OriginalTitle     string            `json:"OriginalTitle"`
	SeriesName        string            `json:"SeriesName"`
	Type              string            `json:"Type"`
	ProductionYear    int               `json:"ProductionYear"`
	RunTimeTicks      int64             `json:"RunTimeTicks"`
	ParentIndexNumber int               `json:"ParentIndexNumber"`
	IndexNumber       int               `json:"IndexNumber"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
}

func mapHistoryEvent(row activityRow, item itemResponse) (integrations.HistoryEvent, error) {
	playedAt, err := parsePlayedAt(row.PlayedAt, row.historyDate)
	if err != nil || !strings.EqualFold(row.ItemID, item.ID) || item.RunTimeTicks < 0 {
		return integrations.HistoryEvent{}, integrations.ErrInvalidHistory
	}
	positionSeconds, err := strconv.Atoi(row.Duration)
	if err != nil || positionSeconds < 0 {
		return integrations.HistoryEvent{}, integrations.ErrInvalidHistory
	}
	identity, err := mapIdentity(item)
	if err != nil || row.Type != item.Type {
		return integrations.HistoryEvent{}, integrations.ErrInvalidHistory
	}
	return integrations.HistoryEvent{
		ID: "emby:" + row.RowID, PlayedAt: playedAt,
		DurationSeconds: int(item.RunTimeTicks / ticksPerSecond),
		PositionSeconds: positionSeconds, Item: identity,
	}, nil
}

func mapIdentity(item itemResponse) (integrations.ItemIdentity, error) {
	identity := integrations.ItemIdentity{
		ProviderItemID: item.ID,
		TMDBID:         item.ProviderIDs["Tmdb"],
		IMDbID:         item.ProviderIDs["Imdb"],
		TVDBID:         item.ProviderIDs["Tvdb"],
		Title:          item.Name,
		OriginalTitle:  item.OriginalTitle,
		Year:           item.ProductionYear,
	}
	switch item.Type {
	case "Movie":
		identity.MediaType = integrations.MediaMovie
	case "Episode":
		identity.MediaType = integrations.MediaEpisode
		identity.SeasonNumber = item.ParentIndexNumber
		identity.EpisodeNumber = item.IndexNumber
		if item.SeriesName != "" {
			identity.Title = item.SeriesName
		}
	default:
		return integrations.ItemIdentity{}, integrations.ErrInvalidHistory
	}
	if identity.ProviderItemID == "" || identity.Title == "" ||
		(identity.MediaType == integrations.MediaEpisode &&
			(identity.SeasonNumber < 0 || identity.EpisodeNumber < 0)) {
		return integrations.ItemIdentity{}, integrations.ErrInvalidHistory
	}
	return identity, nil
}

func parsePlayedAt(value string, historyDate time.Time) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	if !historyDate.IsZero() {
		parsed, err := time.ParseInLocation("15:04", value, historyDate.Location())
		if err == nil {
			year, month, day := historyDate.Date()
			return time.Date(
				year, month, day, parsed.Hour(), parsed.Minute(), 0, 0, historyDate.Location(),
			).UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid playback time")
}
