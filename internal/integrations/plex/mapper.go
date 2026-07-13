package plex

import (
	"encoding/xml"
	"strings"
	"time"

	"video-record/internal/integrations"
)

type historyContainer struct {
	XMLName   xml.Name       `xml:"MediaContainer" json:"-"`
	Size      int            `xml:"size,attr" json:"size"`
	TotalSize int            `xml:"totalSize,attr" json:"totalSize"`
	Offset    int            `xml:"offset,attr" json:"offset"`
	Metadata  []historyVideo `xml:"Video" json:"Metadata"`
}

type historyVideo struct {
	HistoryKey       string     `xml:"historyKey,attr" json:"historyKey"`
	RatingKey        string     `xml:"ratingKey,attr" json:"ratingKey"`
	Type             string     `xml:"type,attr" json:"type"`
	Title            string     `xml:"title,attr" json:"title"`
	OriginalTitle    string     `xml:"originalTitle,attr" json:"originalTitle"`
	GrandparentTitle string     `xml:"grandparentTitle,attr" json:"grandparentTitle"`
	Year             int        `xml:"year,attr" json:"year"`
	ParentIndex      int        `xml:"parentIndex,attr" json:"parentIndex"`
	Index            int        `xml:"index,attr" json:"index"`
	Duration         int64      `xml:"duration,attr" json:"duration"`
	ViewOffset       int64      `xml:"viewOffset,attr" json:"viewOffset"`
	ViewedAt         int64      `xml:"viewedAt,attr" json:"viewedAt"`
	GUIDs            []plexGUID `xml:"Guid" json:"Guid"`
}

type plexGUID struct {
	ID string `xml:"id,attr" json:"id"`
}

func mapHistoryEvent(video historyVideo) (integrations.HistoryEvent, error) {
	if video.HistoryKey == "" || video.RatingKey == "" || video.ViewedAt <= 0 ||
		video.Duration < 0 || video.ViewOffset < 0 {
		return integrations.HistoryEvent{}, integrations.ErrInvalidHistory
	}
	identity, err := mapIdentity(video)
	if err != nil {
		return integrations.HistoryEvent{}, err
	}
	return integrations.HistoryEvent{
		ID:              "plex:" + video.HistoryKey,
		PlayedAt:        time.Unix(video.ViewedAt, 0).UTC(),
		DurationSeconds: int(video.Duration / 1000),
		PositionSeconds: int(video.ViewOffset / 1000),
		Item:            identity,
	}, nil
}

func mapIdentity(video historyVideo) (integrations.ItemIdentity, error) {
	identity := integrations.ItemIdentity{
		ProviderItemID: video.RatingKey,
		Title:          video.Title,
		OriginalTitle:  video.OriginalTitle,
		Year:           video.Year,
	}
	for _, guid := range video.GUIDs {
		assignGUID(&identity, guid.ID)
	}
	switch video.Type {
	case "movie":
		identity.MediaType = integrations.MediaMovie
	case "episode":
		identity.MediaType = integrations.MediaEpisode
		identity.SeasonNumber = video.ParentIndex
		identity.EpisodeNumber = video.Index
		if video.GrandparentTitle != "" {
			identity.Title = video.GrandparentTitle
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

func assignGUID(identity *integrations.ItemIdentity, value string) {
	value, _, _ = strings.Cut(value, "?")
	for prefix, destination := range map[string]*string{
		"tmdb://":                          &identity.TMDBID,
		"imdb://":                          &identity.IMDbID,
		"tvdb://":                          &identity.TVDBID,
		"com.plexapp.agents.themoviedb://": &identity.TMDBID,
		"com.plexapp.agents.imdb://":       &identity.IMDbID,
		"com.plexapp.agents.thetvdb://":    &identity.TVDBID,
	} {
		if strings.HasPrefix(value, prefix) {
			*destination = strings.TrimPrefix(value, prefix)
			return
		}
	}
}
