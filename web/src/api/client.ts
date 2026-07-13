import type {
  CalendarFilter,
  CalendarResponse,
  BackupArtifact,
  CurrentUser,
  HouseholdMember,
  ImportReport,
  LibraryResponse,
  MediaDetails,
  MediaSearchResult,
  MediaType,
  RecordState,
  RecordStatus,
  SeriesProgress,
  StatsSummary,
  SyncCandidate,
  SyncStatusResponse,
  SearchResultsResponse,
  RestoreResult,
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
  participantIds?: string[]
}

export async function updateRecord(mediaID: string, version: number, payload: UpdateRecordPayload): Promise<RecordState> {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<RecordState>(`/api/v1/records/${encodeURIComponent(mediaID)}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
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

export function getEpisodeProgress(mediaID: string, signal?: AbortSignal) {
  return requestJSON<SeriesProgress>(
    `/api/v1/records/${encodeURIComponent(mediaID)}/progress`,
    signal ? { signal } : undefined,
  )
}

export function getCalendar(month: string, timezone: string, filter: CalendarFilter, signal?: AbortSignal) {
  const query = new URLSearchParams({ month, timezone, filter })
  return requestJSON<CalendarResponse>(`/api/v1/calendar?${query.toString()}`, signal ? { signal } : undefined)
}

export function getStats(timezone: string, signal?: AbortSignal) {
  const query = new URLSearchParams({ timezone })
  return requestJSON<StatsSummary>(`/api/v1/stats?${query.toString()}`, signal ? { signal } : undefined)
}

export function getCurrentUser(signal?: AbortSignal) {
  return requestJSON<CurrentUser>('/api/v1/auth/me', signal ? { signal } : undefined)
}

export function getHouseholdMembers(signal?: AbortSignal) {
  return requestJSON<HouseholdMember[]>('/api/v1/household/members', signal ? { signal } : undefined)
}

export function getHouseholdParticipants(signal?: AbortSignal) {
  return requestJSON<HouseholdMember[]>('/api/v1/household/participants', signal ? { signal } : undefined)
}

export function createHouseholdMember(username: string, password: string) {
  return householdWrite<HouseholdMember>('/api/v1/household/members', { username, password })
}

export function resetHouseholdMemberPassword(memberID: string, password: string) {
  return householdWrite<void>(`/api/v1/household/members/${encodeURIComponent(memberID)}/reset-password`, { password })
}

export function deactivateHouseholdMember(memberID: string) {
  return householdWrite<void>(`/api/v1/household/members/${encodeURIComponent(memberID)}/deactivate`, {})
}

export function importData(file: File) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  const body = new FormData()
  body.set('file', file)
  return requestJSON<ImportReport>('/api/v1/data/import', {
    method: 'POST',
    headers: {
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body,
  })
}

export function getBackups(signal?: AbortSignal) {
  return requestJSON<BackupArtifact[]>('/api/v1/backups', signal ? { signal } : undefined)
}

export function getSyncStatus(signal?: AbortSignal) {
  return requestJSON<SyncStatusResponse>('/api/v1/sync/status', signal ? { signal } : undefined)
}

export function getSyncCandidates(signal?: AbortSignal) {
  return requestJSON<SyncCandidate[]>('/api/v1/sync/candidates', signal ? { signal } : undefined)
}

export function confirmSyncCandidate(candidateID: string) {
  return protectedWrite<SyncCandidate>(
    `/api/v1/sync/candidates/${encodeURIComponent(candidateID)}/confirm`,
    {},
  )
}

export function rematchSyncCandidate(candidateID: string, mediaID: string, episodeID = '') {
  return protectedWrite<SyncCandidate>(
    `/api/v1/sync/candidates/${encodeURIComponent(candidateID)}/rematch`,
    { mediaId: mediaID, episodeId: episodeID },
  )
}

export function ignoreSyncCandidate(candidateID: string) {
  return protectedWrite<SyncCandidate>(
    `/api/v1/sync/candidates/${encodeURIComponent(candidateID)}/ignore`,
    {},
  )
}

export function createCustomSyncCandidate(
  candidateID: string,
  payload: { title: string; mediaType: MediaType; year: string },
) {
  return protectedWrite<SyncCandidate>(
    `/api/v1/sync/candidates/${encodeURIComponent(candidateID)}/custom`,
    payload,
  )
}

export function createBackup() {
  return protectedWrite<BackupArtifact>('/api/v1/backups', {})
}

export function restoreBackup(file: File) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  const body = new FormData()
  body.set('file', file)
  return requestJSON<RestoreResult>('/api/v1/restore', {
    method: 'POST',
    headers: {
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body,
  })
}

export type UpdateEpisodeProgressPayload = {
  action: 'single' | 'range' | 'season' | 'next' | 'undo'
  expectedVersion: number
  episodeId?: string
  throughEpisodeId?: string
  seasonId?: string
  watchedAt?: string
}

export function updateEpisodeProgress(mediaID: string, payload: UpdateEpisodeProgressPayload) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<SeriesProgress>(`/api/v1/records/${encodeURIComponent(mediaID)}/progress`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify(payload),
  })
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
    headers: {
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
  })
}

function createIdempotencyKey() {
  return typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function householdWrite<T>(path: string, body: unknown) {
  return protectedWrite<T>(path, body)
}

function protectedWrite<T>(path: string, body: unknown) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<T>(path, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify(body),
  })
}
