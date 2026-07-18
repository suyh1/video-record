import type {
  CalendarFilter,
  CalendarResponse,
  BackupArtifact,
  Collection,
  CurrentUser,
	CurrentRound,
  HouseholdMember,
  ImportReport,
  IntegrationAccount,
  CreateIntegrationAccountPayload,
  LibraryResponse,
  MediaDetails,
  MediaSearchResult,
  MediaType,
  RecordState,
  RecordTags,
  RecordSharing,
  RecordStatus,
	RewatchRoundResult,
	RoundDetail,
	RoundSummary,
  SeriesProgress,
  StatsSummary,
  SyncCandidate,
  SyncStatusResponse,
  SearchResultsResponse,
  RestoreResult,
  SetupStatus,
  TMDBSearchResponse,
  TMDBCastMember,
  TMDBHighlight,
  TMDBMovieDetails,
  TMDBSeasonDetails,
  TMDBTVDetails,
  EpisodeReference,
  VisibleHouseholdRecord,
  LoginResponse,
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
  const response = await request(path, init)
  if (response.status === 204) return undefined as T
  return (await response.json()) as T
}

async function request(path: string, init?: RequestInit) {
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
  return response
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

export async function getTMDBHighlights(signal?: AbortSignal): Promise<TMDBHighlight[]> {
  const response = await requestJSON<{ items: TMDBHighlight[] }>(
    '/api/v1/public/tmdb/highlights',
    signal ? { signal } : undefined,
  )
  return response.items
}

export function testTMDBConnectivity() {
  return requestJSON<{ connected: boolean }>('/api/v1/tmdb/connectivity')
}

export function getTMDBMovie(id: number, signal?: AbortSignal) {
  return requestJSON<TMDBMovieDetails>(`/api/v1/tmdb/movie/${id}`, signal ? { signal } : undefined)
}

export function getTMDBTV(id: number, signal?: AbortSignal) {
  return requestJSON<TMDBTVDetails>(`/api/v1/tmdb/tv/${id}`, signal ? { signal } : undefined)
}

export function getTMDBSeason(id: number, seasonNumber: number, signal?: AbortSignal) {
  return requestJSON<TMDBSeasonDetails>(
    `/api/v1/tmdb/tv/${id}/season/${seasonNumber}`,
    signal ? { signal } : undefined,
  )
}

export async function getTMDBCredits(mediaType: MediaType, id: number, signal?: AbortSignal) {
  const response = await requestJSON<{ cast: TMDBCastMember[] }>(
    `/api/v1/tmdb/${mediaType}/${id}/credits`,
    signal ? { signal } : undefined,
  )
  return response.cast
}

export type UpdateCurrentRoundPayload = {
	status: RecordStatus
	rating?: number | null
	note?: string | null
	viewingMethod?: string | null
	watchedAt?: string
	startedAt?: string | null
	participantIds?: string[]
}

function roundScopeQuery(seasonNumber?: number) {
	if (seasonNumber === undefined) return ''
	if (!Number.isInteger(seasonNumber) || seasonNumber < 1) throw new Error('Invalid season number')
	return `?seasonNumber=${seasonNumber}`
}

export function getCurrentRound(mediaID: string, seasonNumber?: number, signal?: AbortSignal) {
	return requestJSON<CurrentRound>(
		`/api/v1/records/${encodeURIComponent(mediaID)}/rounds/current${roundScopeQuery(seasonNumber)}`,
		signal ? { signal } : undefined,
	)
}

export function updateCurrentRound(
	mediaID: string,
	seasonNumber: number | undefined,
	version: number,
	payload: UpdateCurrentRoundPayload,
) {
	const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
	return requestJSON<CurrentRound>(
		`/api/v1/records/${encodeURIComponent(mediaID)}/rounds/current${roundScopeQuery(seasonNumber)}`,
		{
			method: 'PUT',
			headers: {
				'Content-Type': 'application/json',
				'Idempotency-Key': createIdempotencyKey(),
				'If-Match': `"${version}"`,
				'X-CSRF-Token': csrfToken,
			},
			body: JSON.stringify(payload),
		},
	)
}

export async function getRoundHistory(mediaID: string, seasonNumber?: number, signal?: AbortSignal) {
	const response = await requestJSON<{ rounds: RoundSummary[] }>(
		`/api/v1/records/${encodeURIComponent(mediaID)}/rounds${roundScopeQuery(seasonNumber)}`,
		signal ? { signal } : undefined,
	)
	return response.rounds
}

export function getRoundDetail(mediaID: string, roundID: string, seasonNumber?: number, signal?: AbortSignal) {
	return requestJSON<RoundDetail>(
		`/api/v1/records/${encodeURIComponent(mediaID)}/rounds/${encodeURIComponent(roundID)}${roundScopeQuery(seasonNumber)}`,
		signal ? { signal } : undefined,
	)
}

export function startRewatch(mediaID: string, seasonNumber: number | undefined, version: number) {
	const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
	return requestJSON<RewatchRoundResult>(
		`/api/v1/records/${encodeURIComponent(mediaID)}/rounds/current/rewatch${roundScopeQuery(seasonNumber)}`,
		{
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'Idempotency-Key': createIdempotencyKey(),
				'If-Match': `"${version}"`,
				'X-CSRF-Token': csrfToken,
			},
			body: JSON.stringify({}),
		},
	)
}

export function getMedia(mediaID: string, signal?: AbortSignal) {
  return requestJSON<MediaDetails>(`/api/v1/media/${encodeURIComponent(mediaID)}`, signal ? { signal } : undefined)
}

export function getRecord(mediaID: string, signal?: AbortSignal) {
  return requestJSON<RecordState>(`/api/v1/records/${encodeURIComponent(mediaID)}`, signal ? { signal } : undefined)
}

export function getRecordTags(mediaID: string, signal?: AbortSignal) {
  return requestJSON<RecordTags>(
    `/api/v1/records/${encodeURIComponent(mediaID)}/tags`,
    signal ? { signal } : undefined,
  )
}

export async function setRecordTags(mediaID: string, version: number, tags: string[]) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  const response = await request(`/api/v1/records/${encodeURIComponent(mediaID)}/tags`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'If-Match': `"${version}"`,
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ tags }),
  })
  const nextVersion = Number(response.headers.get('ETag')?.replaceAll('"', ''))
  if (!Number.isInteger(nextVersion) || nextVersion < 0) throw new Error('Invalid record ETag')
  return nextVersion
}

export function getRecordSharing(mediaID: string, signal?: AbortSignal) {
  return requestJSON<RecordSharing>(
    `/api/v1/household/records/${encodeURIComponent(mediaID)}/sharing`,
    signal ? { signal } : undefined,
  )
}

export function updateRecordSharing(
  mediaID: string,
  payload: { shareRating: boolean; shareReview: boolean; sharedReview: string; expectedVersion: number },
) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<RecordSharing>(`/api/v1/household/records/${encodeURIComponent(mediaID)}/sharing`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify(payload),
  })
}

export function getVisibleHouseholdRecord(ownerID: string, mediaID: string, signal?: AbortSignal) {
  return requestJSON<VisibleHouseholdRecord>(
    `/api/v1/household/records/${encodeURIComponent(ownerID)}/${encodeURIComponent(mediaID)}`,
    signal ? { signal } : undefined,
  )
}

export function getEpisodeProgress(mediaID: string, seasonNumber: number, signal?: AbortSignal) {
	return requestJSON<SeriesProgress>(
		`/api/v1/records/${encodeURIComponent(mediaID)}/progress${roundScopeQuery(seasonNumber)}`,
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

export function getSetupStatus(signal?: AbortSignal) {
  return requestJSON<SetupStatus>('/api/v1/setup/status', signal ? { signal } : undefined)
}

export function initializeAdministrator(username: string, password: string) {
  return requestJSON<CurrentUser>('/api/v1/setup/admin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
}

export function loginUser(username: string, password: string) {
  return requestJSON<LoginResponse>('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
}

export function logoutUser() {
  return requestJSON<void>('/api/v1/auth/logout', {
    method: 'POST',
  })
}

export function changePassword(currentPassword: string, newPassword: string) {
  return protectedWrite<void>('/api/v1/auth/password', { currentPassword, newPassword })
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

export function getIntegrationAccounts(signal?: AbortSignal) {
  return requestJSON<IntegrationAccount[]>('/api/v1/integrations/accounts', signal ? { signal } : undefined)
}

export function createIntegrationAccount(payload: CreateIntegrationAccountPayload) {
  return protectedWrite<IntegrationAccount>('/api/v1/integrations/accounts', payload)
}

export function disconnectIntegrationAccount(accountID: string) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<void>(`/api/v1/integrations/accounts/${encodeURIComponent(accountID)}`, {
    method: 'DELETE',
    headers: {
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
  })
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
	action: 'single' | 'range' | 'season' | 'next' | 'undo' | 'set_time'
  expectedVersion: number
  episodeId?: string
  throughEpisodeId?: string
  seasonId?: string
  watchedAt?: string
  episodeRefs?: EpisodeReference[]
  totalEpisodes?: number
}

export function updateEpisodeProgress(
	mediaID: string,
	seasonNumber: number,
	payload: UpdateEpisodeProgressPayload,
) {
	const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
	return requestJSON<SeriesProgress>(`/api/v1/records/${encodeURIComponent(mediaID)}/progress${roundScopeQuery(seasonNumber)}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify(payload),
  })
}

export type GetLibraryOptions = {
  cursor?: string | null
  limit?: number
  mediaType?: 'movie' | 'tv' | 'all'
  sort?: 'updated' | 'title' | 'rating' | 'watched'
  q?: string
  tag?: string
  signal?: AbortSignal
}

export function getLibrary(
  status: RecordStatus | 'all' = 'all',
  signalOrOptions?: AbortSignal | GetLibraryOptions,
) {
  const options: GetLibraryOptions = signalOrOptions instanceof AbortSignal || signalOrOptions === undefined
    ? { ...(signalOrOptions ? { signal: signalOrOptions } : {}) }
    : signalOrOptions
  const params = new URLSearchParams()
  if (status !== 'all') params.set('status', status)
  if (options.cursor) params.set('cursor', options.cursor)
  if (options.limit !== undefined) params.set('limit', String(options.limit))
  if (options.mediaType && options.mediaType !== 'all') params.set('mediaType', options.mediaType)
  if (options.sort && options.sort !== 'updated') params.set('sort', options.sort)
  if (options.q) params.set('q', options.q)
  if (options.tag) params.set('tag', options.tag)
  const query = params.toString()
  return requestJSON<LibraryResponse>(
    `/api/v1/library${query ? `?${query}` : ''}`,
    options.signal ? { signal: options.signal } : undefined,
  )
}

export function getCollections(signal?: AbortSignal) {
  return requestJSON<Collection[]>('/api/v1/collections', signal ? { signal } : undefined)
}

export function getCollectionItems(
  collectionID: string,
  options?: { status?: RecordStatus | 'all'; signal?: AbortSignal },
) {
  const params = new URLSearchParams()
  if (options?.status && options.status !== 'all') params.set('status', options.status)
  const query = params.toString()
  return requestJSON<LibraryResponse>(
    `/api/v1/collections/${encodeURIComponent(collectionID)}/items${query ? `?${query}` : ''}`,
    options?.signal ? { signal: options.signal } : undefined,
  )
}

export function getUserTags(signal?: AbortSignal) {
  return requestJSON<{ tags: string[] }>('/api/v1/tags', signal ? { signal } : undefined)
}

export async function getViewingMethods(signal?: AbortSignal) {
  const response = await requestJSON<{ methods: string[] }>(
    '/api/v1/records/viewing-methods',
    signal ? { signal } : undefined,
  )
  return response.methods

}

export function renameCollection(collectionID: string, name: string) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<Collection>(`/api/v1/collections/${encodeURIComponent(collectionID)}`, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ name }),
  })
}

export function deleteCollection(collectionID: string) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<void>(`/api/v1/collections/${encodeURIComponent(collectionID)}`, {
    method: 'DELETE',
    headers: {
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
  })
}

export function createCollection(name: string) {
  return protectedWrite<Collection>('/api/v1/collections', { name })
}

export function addCollectionItem(collectionID: string, mediaID: string) {
  return protectedWrite<void>(`/api/v1/collections/${encodeURIComponent(collectionID)}/items`, { mediaId: mediaID })
}

export function replaceCollectionItems(collectionID: string, mediaIDs: string[]) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<void>(`/api/v1/collections/${encodeURIComponent(collectionID)}/items`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({ mediaIds: mediaIDs }),
  })
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

/** Materialize TMDB items when needed, then set movie/TV profile status for one-tap search recording. */
export async function quickRecordMedia(
  item: MediaSearchResult,
  status: Extract<RecordStatus, 'wishlist' | 'completed'>,
) {
  let mediaID = item.id
  if (item.source === 'tmdb') {
    const imported = await createMediaFromTMDB(item)
    mediaID = imported.id
  }
  const payload: UpdateCurrentRoundPayload = { status }
  if (status === 'completed') payload.watchedAt = new Date().toISOString()
  // Movies use media-scope current round; TV wishlist updates projected profile via seasonless path when applicable.
  const seasonNumber = item.mediaType === 'tv' ? 1 : undefined
  const current = await getCurrentRound(mediaID, seasonNumber)
  await updateCurrentRound(mediaID, seasonNumber, current.version, payload)
  return mediaID
}

export async function linkMediaToTMDB(mediaID: string, item: MediaSearchResult): Promise<MediaDetails> {
  if (item.source !== 'tmdb' || !item.externalId) throw new Error('TMDB identity required')
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<MediaDetails>(
    `/api/v1/media/${encodeURIComponent(mediaID)}/tmdb/${item.mediaType}/${item.externalId}`,
    {
      method: 'POST',
      headers: {
        'Idempotency-Key': createIdempotencyKey(),
        'X-CSRF-Token': csrfToken,
      },
    },
  )
}

export function createCustomMedia(payload: { title: string; mediaType: MediaType; year: string; overview: string }) {
  const csrfToken = sessionStorage.getItem('video-record.csrf-token') ?? ''
  return requestJSON<MediaDetails>('/api/v1/media/custom', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': createIdempotencyKey(),
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify(payload),
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
