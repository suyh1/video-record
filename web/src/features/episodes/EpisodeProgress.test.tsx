import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, expect, it } from 'vitest'

import type { SeriesProgress } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { EpisodeProgress } from './EpisodeProgress'

const sparseProgress: SeriesProgress = {
  mediaId: 'series-1',
  status: 'watching',
  version: 1,
  watchedEpisodes: 1,
  totalEpisodes: 1,
  lastWatched: { id: 'local-101', sourceId: '101', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 1, absoluteNumber: 1, name: '', watched: true, watchedAt: '2026-07-12T12:00:00Z' },
  nextEpisode: null,
  episodes: [
    { id: 'local-101', sourceId: '101', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 1, absoluteNumber: 1, name: '', watched: true, watchedAt: '2026-07-12T12:00:00Z' },
  ],
}

const tvDetails = {
  id: 1399, name: '测试剧集', originalName: 'Test Series', firstAirDate: '2026-01-01',
  posterPath: '', backdropPath: '', overview: '', numberOfSeasons: 2, numberOfEpisodes: 6,
  episodeRuntime: [45], genres: ['剧情'],
  seasons: [
    { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: 1, episodeCount: 3 },
    { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 3 },
  ],
}

const liveSeasons = {
  1: {
    id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: 1,
    episodes: [
      { id: 101, name: '第一集', overview: '', airDate: '2026-01-01', seasonNumber: 1, episodeNumber: 1, runtime: 45, stillPath: '' },
      { id: 102, name: '第二集', overview: '', airDate: '2026-01-08', seasonNumber: 1, episodeNumber: 2, runtime: 45, stillPath: '' },
      { id: 103, name: '第三集', overview: '', airDate: '2026-01-15', seasonNumber: 1, episodeNumber: 3, runtime: 45, stillPath: '' },
    ],
  },
  2: {
    id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2,
    episodes: [
      { id: 201, name: '第四集', overview: '', airDate: '2026-07-01', seasonNumber: 2, episodeNumber: 1, runtime: 45, stillPath: '' },
      { id: 202, name: '第五集', overview: '', airDate: '2026-07-08', seasonNumber: 2, episodeNumber: 2, runtime: 45, stillPath: '' },
      { id: 203, name: '第六集', overview: '', airDate: '2026-07-15', seasonNumber: 2, episodeNumber: 3, runtime: 45, stillPath: '' },
    ],
  },
}

beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

function useLiveCatalog() {
  server.use(
    http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json(sparseProgress)),
    http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json(tvDetails)),
    http.get('*/api/v1/tmdb/tv/1399/season/:season', ({ params }) =>
      HttpResponse.json(liveSeasons[Number(params.season) as 1 | 2])),
  )
}

it('loads one live season, shows absolute labels, and switches seasons', async () => {
  useLiveCatalog()
  const user = userEvent.setup()
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={1399} />)

  expect(await screen.findByRole('button', { name: '标记 S01E02 已看' })).toHaveTextContent('全剧第 2 集')
  expect(screen.getByText('1 / 6 集')).toBeVisible()
  await user.selectOptions(screen.getByRole('combobox', { name: '选择季' }), '2')
  expect(await screen.findByRole('button', { name: '标记 S02E02 已看' })).toHaveTextContent('全剧第 5 集')
  expect(screen.queryByText('第二集')).not.toBeInTheDocument()
})

it('advances a live next episode and sends only sparse identity data', async () => {
  useLiveCatalog()
  let savedBody: Record<string, unknown> | undefined
  server.use(http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
    savedBody = await request.json() as Record<string, unknown>
    expect(request.headers.get('Idempotency-Key')).toBeTruthy()
    expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
    return HttpResponse.json({
      ...sparseProgress,
      version: 2,
      watchedEpisodes: 2,
      totalEpisodes: 6,
      lastWatched: { id: 'local-102', sourceId: '102', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2, name: '', watched: true, watchedAt: '2026-07-14T12:00:00Z' },
      episodes: [
        ...sparseProgress.episodes,
        { id: 'local-102', sourceId: '102', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2, name: '', watched: true, watchedAt: '2026-07-14T12:00:00Z' },
      ],
    })
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={1399} />)

  await user.click(await screen.findByRole('button', { name: '推进下一集 S01E02' }))

  await waitFor(() => expect(savedBody).toMatchObject({
    action: 'next', expectedVersion: 1, totalEpisodes: 6,
    episodeRefs: [{ sourceId: '102', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2 }],
  }))
  expect(savedBody).not.toHaveProperty('name')
  expect(await screen.findByRole('status')).toHaveTextContent('已推进至 S01E02')
  expect(screen.getByRole('button', { name: '撤销 S01E02' })).toBeVisible()
})

it('retries a failed live season without hiding saved progress', async () => {
  let requests = 0
  server.use(
    http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json(sparseProgress)),
    http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json(tvDetails)),
    http.get('*/api/v1/tmdb/tv/1399/season/1', () => {
      requests += 1
      return requests === 1 ? HttpResponse.json({ code: 'tmdb_unavailable' }, { status: 502 }) : HttpResponse.json(liveSeasons[1])
    }),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={1399} />)

  expect(await screen.findByRole('alert')).toHaveTextContent('无法获取第 1 季分集资料')
  expect(screen.getByText('已记录 1 集')).toBeVisible()
  await user.click(screen.getByRole('button', { name: '重新获取分集资料' }))
  expect(await screen.findByText('第二集')).toBeVisible()
})

it('keeps a local compatibility path for an unlinked series', async () => {
  server.use(http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json({
    ...sparseProgress,
    totalEpisodes: 2,
    episodes: [
      sparseProgress.episodes[0],
      { id: 'local-102', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2, name: '本地第二集', watched: false, watchedAt: null },
    ],
  })))
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={null} />)

  expect(await screen.findByRole('button', { name: '标记 S01E02 已看' })).toHaveTextContent('本地第二集')
})
