package tmdb

type SearchResponse struct {
	Page         int            `json:"page"`
	Results      []SearchResult `json:"results"`
	TotalPages   int            `json:"total_pages"`
	TotalResults int            `json:"total_results"`
}

type SearchResult struct {
	ID            int    `json:"id"`
	MediaType     string `json:"media_type"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	OriginalTitle string `json:"original_title"`
	OriginalName  string `json:"original_name"`
	ReleaseDate   string `json:"release_date"`
	FirstAirDate  string `json:"first_air_date"`
	PosterPath    string `json:"poster_path"`
	Overview      string `json:"overview"`
}

type MovieDetails struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	ReleaseDate   string  `json:"release_date"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	Overview      string  `json:"overview"`
	Runtime       int     `json:"runtime"`
	Genres        []Genre `json:"genres"`
}

type TVDetails struct {
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	OriginalName   string          `json:"original_name"`
	FirstAirDate   string          `json:"first_air_date"`
	PosterPath     string          `json:"poster_path"`
	BackdropPath   string          `json:"backdrop_path"`
	Overview       string          `json:"overview"`
	NumberSeasons  int             `json:"number_of_seasons"`
	NumberEpisodes int             `json:"number_of_episodes"`
	EpisodeRunTime []int           `json:"episode_run_time"`
	Genres         []Genre         `json:"genres"`
	Seasons        []SeasonSummary `json:"seasons"`
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type SeasonDetails struct {
	ID           int              `json:"id"`
	Name         string           `json:"name"`
	Overview     string           `json:"overview"`
	PosterPath   string           `json:"poster_path"`
	AirDate      string           `json:"air_date"`
	SeasonNumber int              `json:"season_number"`
	Episodes     []EpisodeDetails `json:"episodes"`
}

type SeasonSummary struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"poster_path"`
	AirDate      string `json:"air_date"`
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
}

type EpisodeDetails struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Runtime       int    `json:"runtime"`
	StillPath     string `json:"still_path"`
}

type CastMember struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
}

type Credits struct {
	Cast []CastMember `json:"cast"`
}
