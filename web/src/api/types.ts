export type MediaType = 'movie' | 'tv'
export type RecordStatus = 'none' | 'wishlist' | 'watching' | 'completed' | 'dropped'

export type MediaSearchResult = {
  id: string
  source: 'local' | 'tmdb'
  externalId?: number
  mediaType: MediaType
  title: string
  originalTitle: string
  year: string
  posterPath: string | null
  status: RecordStatus
}

export type SearchResultsResponse = {
  items: MediaSearchResult[]
}

export type RecordState = {
  mediaId: string
  status: RecordStatus
  rating: number | null
  note: string | null
  watchedAt: string | null
  viewingMethod: string | null
  version: number
}

export type MediaDetails = {
  id: string
  mediaType: MediaType
  title: string
  originalTitle: string
  releaseDate: string
  overview: string
  posterPath: string | null
  backdropPath: string | null
}

export type WatchEvent = {
  id: string
  mediaId: string
  episodeId?: string
  watchedAt: string
  viewingMethod?: string
  source: 'manual' | 'confirmed_import' | 'confirmed_sync' | 'external_default'
  externalEventId?: string
  completion: number
  note?: string
}

export type EpisodeProgressItem = {
  id: string
  seasonId: string
  seasonNumber: number
  episodeNumber: number
  absoluteNumber: number
  name: string
  watched: boolean
  watchedAt: string | null
}

export type SeriesProgress = {
  mediaId: string
  status: RecordStatus
  version: number
  watchedEpisodes: number
  totalEpisodes: number
  lastWatched: EpisodeProgressItem | null
  nextEpisode: EpisodeProgressItem | null
  episodes: EpisodeProgressItem[]
}

export type CalendarFilter = 'all' | 'completed' | 'in_progress'

export type CalendarEvent = {
  id: string
  mediaId: string
  mediaType: MediaType
  title: string
  episodeId: string | null
  seasonNumber: number | null
  episodeNumber: number | null
  absoluteNumber: number | null
  watchedAt: string
  localDate: string
  viewingMethod: string | null
  participants: string[]
  status: RecordStatus
}

export type CalendarResponse = {
  year: number
  month: number
  timezone: string
  events: CalendarEvent[]
}

export type StatsPoint = {
  label: string
  value: number
}

export type StatsSummary = {
  totalWatches: number
  uniqueMedia: number
  totalMinutes: number
  repeatWatches: number
  monthly: StatsPoint[]
  yearly: StatsPoint[]
  genres: StatsPoint[]
  ratings: StatsPoint[]
  tags: StatsPoint[]
  viewingMethods: StatsPoint[]
}

export type CurrentUser = {
  id: string
  username: string
  role: 'admin' | 'member'
}

export type HouseholdMember = CurrentUser & {
  active: boolean
  createdAt?: string
}

export type ImportReport = {
  importedRecords: number
  importedCollections: number
  failures: Array<{
    recordId: string
    code: string
  }>
}

export type LibraryResponse = {
  items: MediaSearchResult[]
  nextCursor: string | null
}

export type TMDBSearchResponse = {
  results: Array<{
    id: number
    mediaType: MediaType
    title: string
    originalTitle: string
    year?: string
    releaseDate?: string
    posterPath: string | null
  }>
}
