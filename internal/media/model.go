package media

type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
)

type Item struct {
	ID               string
	TMDBID           *int
	MediaType        MediaType
	Title            string
	Overview         string
	ExternalTitle    string
	ExternalOverview string
	OriginalTitle    string
	ReleaseDate      string
	PosterPath       string
	BackdropPath     string
	RuntimeMinutes   int
	Genres           []string
}

type ExternalGenre struct {
	ID   string
	Name string
}

type ExternalSnapshot struct {
	Source         string
	SourceID       string
	MediaType      MediaType
	Title          string
	OriginalTitle  string
	ReleaseDate    string
	Overview       string
	PosterPath     string
	BackdropPath   string
	RuntimeMinutes int
	Genres         []ExternalGenre
}

type CreateCustomInput struct {
	MediaType MediaType
	Title     string
	Overview  string
	Year      string
}
