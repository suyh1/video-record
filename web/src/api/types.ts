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
