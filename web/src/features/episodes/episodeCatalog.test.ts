import { describe, expect, it } from 'vitest'

import type { SeriesProgress, TMDBSeasonDetails, TMDBSeasonSummary } from '../../api/types'
import { findNextEpisode, mergeSeason, selectActiveSeason, selectDefaultSeason, totalEpisodeCount } from './episodeCatalog'

const seasons: TMDBSeasonSummary[] = [
  { id: 10, name: '特别篇', overview: '', posterPath: '', airDate: '', seasonNumber: 0, episodeCount: 2 },
  { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: 1, episodeCount: 3 },
  { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 2 },
]

const season: TMDBSeasonDetails = {
  id: 12,
  name: '第 2 季',
  overview: '',
  posterPath: '',
  airDate: '2026-07-01',
  seasonNumber: 2,
  episodes: [
    { id: 201, name: '重逢', overview: '', airDate: '2026-07-01', seasonNumber: 2, episodeNumber: 1, runtime: 45, stillPath: '/201.jpg' },
    { id: 202, name: '回声', overview: '', airDate: '2026-07-08', seasonNumber: 2, episodeNumber: 2, runtime: 46, stillPath: '' },
  ],
}

function progress(episodes: SeriesProgress['episodes']): SeriesProgress {
  return {
    roundId: 'round-1',
    mediaId: 'series-1',
    seasonNumber: 1,
    status: 'watching',
    version: 3,
    watchedEpisodes: episodes.filter((episode) => episode.watched).length,
    totalEpisodes: episodes.length,
    lastWatched: episodes.at(-1) ?? null,
    nextEpisode: null,
    episodes,
  }
}

describe('episodeCatalog', () => {
  it('selects the highest currently watching season before other rounds', () => {
    expect(selectActiveSeason(seasons, [
      { seasonNumber: 1, status: 'completed' },
      { seasonNumber: 2, status: 'watching' },
    ])).toBe(2)
    expect(selectActiveSeason(seasons, [])).toBe(1)
  })

  it('merges a live season with sparse progress by source id and legacy numbers', () => {
    const merged = mergeSeason(season, seasons, progress([
      { id: 'local-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1, absoluteNumber: 4, name: '', watched: true, watchedAt: '2026-07-01T12:00:00Z' },
      { id: 'legacy-202', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 2, absoluteNumber: 5, name: '', watched: true, watchedAt: '2026-07-08T12:00:00Z' },
    ]))

    expect(merged.episodes.map((episode) => episode.watched)).toEqual([true, true])
    expect(merged.episodes.map((episode) => episode.absoluteNumber)).toEqual([4, 5])
    expect(merged.episodes[0]?.localId).toBe('local-201')
  })

  it('selects the last watched incomplete season and then the first incomplete season', () => {
    const oneWatched = progress([
      { id: 'local-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1, absoluteNumber: 4, name: '', watched: true, watchedAt: '2026-07-01T12:00:00Z' },
    ])
    expect(selectDefaultSeason(seasons, oneWatched)).toBe(2)

    const secondComplete = progress([
      { id: 'local-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1, absoluteNumber: 4, name: '', watched: true, watchedAt: '2026-07-01T12:00:00Z' },
      { id: 'local-202', sourceId: '202', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 2, absoluteNumber: 5, name: '', watched: true, watchedAt: '2026-07-08T12:00:00Z' },
    ])
    expect(selectDefaultSeason(seasons, secondComplete)).toBe(1)
    expect(totalEpisodeCount(seasons)).toBe(5)
  })

  it('finds the next unwatched episode', () => {
    const merged = mergeSeason(season, seasons, progress([
      { id: 'local-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1, absoluteNumber: 4, name: '', watched: true, watchedAt: '2026-07-01T12:00:00Z' },
    ]))
    expect(findNextEpisode(merged.episodes)?.id).toBe(202)
  })
})
