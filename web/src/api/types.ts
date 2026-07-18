export type MediaType = 'movie' | 'tv'
export type RecordStatus = 'none' | 'wishlist' | 'watching' | 'completed' | 'dropped'

export type MediaSearchResult = {
  id: string
  source: 'local' | 'tmdb'
  externalId?: number
  tmdbId?: number | null
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

export type CurrentRound = {
	roundId: string
	mediaId: string
	seasonNumber: number | null
	roundNumber: number
	status: RecordStatus
	rating: number | null
	note: string | null
	viewingMethod: string | null
	watchedAt: string | null
	startedAt: string | null
	participantIds: string[]
	version: number
	profileVersion: number
}

export type ArchivedRound = Omit<CurrentRound, 'version' | 'profileVersion' | 'participantIds'> & {
	archivedAt: string | null
}

export type RoundSummary = {
	roundId: string
	mediaId: string
	seasonNumber: number | null
	roundNumber: number
	watchedAt: string | null
	rating: number | null
}

export type RoundEpisode = EpisodeProgressItem

export type RoundDetail = {
	round: ArchivedRound
	episodes: RoundEpisode[]
}

export type RewatchRoundResult = {
	archived: ArchivedRound
	current: CurrentRound
}

export type RecordTags = {
  tags: string[]
}

export type RecordSharing = {
  mediaId: string
  shareRating: boolean
  shareReview: boolean
  sharedReview: string | null
  version: number
}

export type VisibleHouseholdRecord = {
  ownerId: string
  mediaId: string
  rating: number | null
  privateNote: string | null
  sharedReview: string | null
}

export type MediaDetails = {
  id: string
  tmdbId: number | null
  mediaType: MediaType
  title: string
  externalTitle: string
  externalOverview: string
  originalTitle: string
  releaseDate: string
  overview: string
  posterPath: string | null
  backdropPath: string | null
  runtimeMinutes: number
  genres: string[]
}

export type EpisodeProgressItem = {
  id: string
  sourceId?: string
  seasonId: string
  seasonNumber: number
  episodeNumber: number
  absoluteNumber: number
  name: string
  watched: boolean
  watchedAt: string | null
}

export type SeriesProgress = {
	roundId: string
	mediaId: string
	seasonNumber: number
  status: RecordStatus
  version: number
  watchedEpisodes: number
  totalEpisodes: number
  lastWatched: EpisodeProgressItem | null
  nextEpisode: EpisodeProgressItem | null
  episodes: EpisodeProgressItem[]
}

export type TMDBSeasonSummary = {
  id: number
  name: string
  overview: string
  posterPath: string
  airDate: string
  seasonNumber: number
  episodeCount: number
}

export type TMDBEpisodeDetails = {
  id: number
  name: string
  overview: string
  airDate: string
  seasonNumber: number
  episodeNumber: number
  runtime: number
  stillPath: string
}

export type TMDBSeasonDetails = {
  id: number
  name: string
  overview: string
  posterPath: string
  airDate: string
  seasonNumber: number
  episodes: TMDBEpisodeDetails[]
}

export type TMDBTVDetails = {
  id: number
  name: string
  originalName: string
  firstAirDate: string
  posterPath: string
  backdropPath: string
  overview: string
  numberOfSeasons: number
  numberOfEpisodes: number
  episodeRuntime: number[]
  genres: string[]
  seasons: TMDBSeasonSummary[]
}

export type TMDBMovieDetails = {
  id: number
  title: string
  originalTitle: string
  releaseDate: string
  posterPath: string
  backdropPath: string
  overview: string
  runtime: number
  genres: string[]
}

export type TMDBCastMember = {
  id: number
  name: string
  character: string
  profilePath: string
  order: number
}

export type TMDBHighlight = {
  id: number
  mediaType: MediaType
  title: string
  originalTitle: string
  year: string
  overview: string
  backdropURL: string
}

export type EpisodeReference = {
  sourceId: string
  seasonNumber: number
  episodeNumber: number
  absoluteNumber: number
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

export type SetupStatus = {
  initialized: boolean
  storageReady: boolean
  tmdbConfigured: boolean
}

export type LoginResponse = {
  user: CurrentUser
  csrfToken: string
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

export type BackupManifest = {
  formatVersion: number
  schemaVersion: number
  createdAt: string
  databaseSha256: string
  databaseBytes: number
  requiresEncryptionKey: boolean
}

export type BackupArtifact = {
  filename: string
  bytes: number
  manifest: BackupManifest
}

export type RestoreResult = {
  preRestoreBackup: string
  warnings: string[]
}

export type LibraryResponse = {
  items: MediaSearchResult[]
  nextCursor: string | null
}

export type Collection = {
  id: string
  name: string
  items: string[]
}

export type SyncCandidateStatus = 'exact' | 'possible' | 'unmatched' | 'conflict' | 'confirmed' | 'ignored'

export type SyncAccountStatus = {
  id: string
  provider: 'jellyfin' | 'emby' | 'plex'
  name: string
  enabled: boolean
  pendingCandidates: number
  lastRunStatus?: 'running' | 'succeeded' | 'failed'
  lastRunAt?: string
  lastRunSummary?: string
}

export type SyncStatusResponse = {
  accounts: SyncAccountStatus[]
  pendingTotal: number
}

export type IntegrationProvider = 'jellyfin' | 'emby' | 'plex'

export type IntegrationAccount = {
  id: string
  provider: IntegrationProvider
  name: string
  credentialFingerprint: string
  enabled: boolean
  locked: boolean
  createdAt: string
  updatedAt: string
}

export type CreateIntegrationAccountPayload = {
  provider: IntegrationProvider
  name: string
  baseUrl: string
  token: string
  userId?: string
  accountId?: number
  timezone?: string
}

export type SyncMatchEvidence = {
  code: string
  text: string
}

export type SyncMatchOption = {
  mediaId: string
  episodeId?: string
  mediaType: MediaType
  title: string
  originalTitle?: string
  year?: string
}

export type SyncCandidate = {
  id: string
  accountId: string
  externalEventId: string
  status: SyncCandidateStatus
  mediaId?: string
  episodeId?: string
  event: {
    playedAt: string
    durationSeconds: number
    positionSeconds: number
    providerItemId: string
    mediaType: 'movie' | 'episode'
    title: string
    originalTitle?: string
    year?: number
    seasonNumber?: number
    episodeNumber?: number
  }
  evidence: SyncMatchEvidence[]
  options: SyncMatchOption[]
  createdAt: string
  updatedAt: string
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
