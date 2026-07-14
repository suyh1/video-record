import type {
  CurrentRound,
  EpisodeProgressItem,
  SeriesProgress,
  TMDBEpisodeDetails,
  TMDBSeasonDetails,
  TMDBSeasonSummary,
} from '../../api/types'

export type MergedEpisode = TMDBEpisodeDetails & {
  absoluteNumber: number
  localId: string | null
  watched: boolean
  watchedAt: string | null
}

export type MergedSeason = Omit<TMDBSeasonDetails, 'episodes'> & {
  episodes: MergedEpisode[]
}

export function regularSeasons(seasons: TMDBSeasonSummary[]) {
  return seasons
    .filter((season) => season.seasonNumber > 0)
    .slice()
    .sort((left, right) => left.seasonNumber - right.seasonNumber)
}

export function totalEpisodeCount(seasons: TMDBSeasonSummary[]) {
  return regularSeasons(seasons).reduce((total, season) => total + season.episodeCount, 0)
}

export function selectActiveSeason(
  seasons: TMDBSeasonSummary[],
  rounds: Array<Pick<CurrentRound, 'seasonNumber' | 'status'>>,
) {
  const available = regularSeasons(seasons)
  if (available.length === 0) return null
  const availableNumbers = new Set(available.map((season) => season.seasonNumber))
  const watching = rounds
    .filter((round) => round.status === 'watching' && round.seasonNumber !== null && availableNumbers.has(round.seasonNumber))
    .map((round) => round.seasonNumber as number)
    .sort((left, right) => right - left)
  return watching[0] ?? available[0]?.seasonNumber ?? null
}

export function mergeSeason(
  season: TMDBSeasonDetails,
  summaries: TMDBSeasonSummary[],
  progress: SeriesProgress,
): MergedSeason {
  const offset = regularSeasons(summaries)
    .filter((summary) => summary.seasonNumber < season.seasonNumber)
    .reduce((total, summary) => total + summary.episodeCount, 0)
  return {
    ...season,
    episodes: season.episodes.map((episode) => {
      const saved = findSavedEpisode(progress.episodes, episode)
      return {
        ...episode,
        absoluteNumber: offset + episode.episodeNumber,
        localId: saved?.id ?? null,
        watched: saved?.watched ?? false,
        watchedAt: saved?.watchedAt ?? null,
      }
    }),
  }
}

export function selectDefaultSeason(seasons: TMDBSeasonSummary[], progress: SeriesProgress) {
  const available = regularSeasons(seasons)
  if (available.length === 0) return null
  const watchedBySeason = new Map<number, number>()
  for (const episode of progress.episodes) {
    if (!episode.watched) continue
    watchedBySeason.set(episode.seasonNumber, (watchedBySeason.get(episode.seasonNumber) ?? 0) + 1)
  }
  const lastSeasonNumber = progress.lastWatched?.seasonNumber
  if (lastSeasonNumber) {
    const lastSeason = available.find((season) => season.seasonNumber === lastSeasonNumber)
    if (lastSeason && (watchedBySeason.get(lastSeasonNumber) ?? 0) < lastSeason.episodeCount) {
      return lastSeasonNumber
    }
  }
  return available.find((season) => (watchedBySeason.get(season.seasonNumber) ?? 0) < season.episodeCount)?.seasonNumber
    ?? available.at(-1)?.seasonNumber
    ?? null
}

export function findNextEpisode(episodes: MergedEpisode[]) {
  return episodes.find((episode) => !episode.watched) ?? null
}

function findSavedEpisode(progress: EpisodeProgressItem[], episode: TMDBEpisodeDetails) {
  return progress.find((saved) => saved.sourceId
    ? saved.sourceId === String(episode.id)
    : saved.seasonNumber === episode.seasonNumber && saved.episodeNumber === episode.episodeNumber)
}
