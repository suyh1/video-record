import type {
  LibraryResponse,
  MediaDetails,
  MediaSearchResult,
  RecordState,
  RecordStatus,
  SearchResultsResponse,
  TMDBSearchResponse,
  WatchEvent,
} from './types'

export class APIError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly requestId: string,
    readonly etag: string | null,
  ) {
    super(code)
    this.name = 'APIError'
  }
}

export async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin), {
    credentials: 'same-origin',
    ...init,
    headers: { Accept: 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const problem = (await response.json().catch(() => ({}))) as { code?: string; requestId?: string }
    throw new APIError(
      response.status,
      problem.code ?? 'request_failed',
      problem.requestId ?? '',
      response.headers.get('ETag'),
    )
  }
  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

export async function searchLocalMedia(query: string, signal?: AbortSignal): Promise<MediaSearchResult[]> {
  const response = await requestJSON<SearchResultsResponse>(
    `/api/v1/media/search?q=${encodeURIComponent(query)}`,
    signal ? { signal } : undefined,
  )
  return response.items
}

export async function searchTMDB(query: string, signal?: AbortSignal): Promise<MediaSearchResult[]> {
  const response = await requestJSON<TMDBSearchResponse>(
    `/api/v1/tmdb/search?q=${encodeURIComponent(query)}`,
    signal ? { signal } : undefined,
  )
  return response.results.map((item) => ({
    id: `tmdb-${item.mediaType}-${item.id}`,
    externalId: item.id,
    source: 'tmdb',
    mediaType: item.mediaType,
    title: item.title,
    originalTitle: item.originalTitle,
    year: item.year ?? item.releaseDate?.slice(0, 4) ?? '',
    posterPath: item.posterPath,
    status: 'none',
  }))
}

export type UpdateRecordPayload = {
  status: RecordStatus
  rating?: number | null
  note?: string | null
  watchedAt?: string
  viewingMethod?: string | null
}

export async function updateRecord(mediaID: string, version: number, payload: UpdateRecordPayload): Promise<RecordState> {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<RecordState>(`/api/v1/records/${encodeURIComponent(mediaID)}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'If-Match': `"${version}"`,
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify(payload),
  })
}

export function getMedia(mediaID: string, signal?: AbortSignal) {
  return requestJSON<MediaDetails>(`/api/v1/media/${encodeURIComponent(mediaID)}`, signal ? { signal } : undefined)
}

export function getRecord(mediaID: string, signal?: AbortSignal) {
  return requestJSON<RecordState>(`/api/v1/records/${encodeURIComponent(mediaID)}`, signal ? { signal } : undefined)
}

export function getWatchEvents(mediaID: string, signal?: AbortSignal) {
  return requestJSON<WatchEvent[]>(`/api/v1/records/${encodeURIComponent(mediaID)}/events`, signal ? { signal } : undefined)
}

export function getLibrary(status: RecordStatus | 'all', signal?: AbortSignal) {
  const query = status === 'all' ? '' : `?status=${status}`
  return requestJSON<LibraryResponse>(`/api/v1/library${query}`, signal ? { signal } : undefined)
}

export async function createMediaFromTMDB(item: MediaSearchResult): Promise<MediaDetails> {
  if (item.source !== 'tmdb' || !item.externalId) throw new Error('TMDB identity required')
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<MediaDetails>(`/api/v1/media/tmdb/${item.mediaType}/${item.externalId}`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
  })
}
