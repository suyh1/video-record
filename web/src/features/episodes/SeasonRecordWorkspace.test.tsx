import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect, it } from 'vitest'

import type { CurrentRound } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { SeasonRecordWorkspace } from './SeasonRecordWorkspace'

const tvDetails = {
  id: 1399, name: '测试剧集', originalName: 'Test Series', firstAirDate: '2026-01-01',
  posterPath: '', backdropPath: '', overview: '', numberOfSeasons: 2, numberOfEpisodes: 2,
  episodeRuntime: [45], genres: ['剧情'],
  seasons: [
    { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: 1, episodeCount: 1 },
    { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 1 },
  ],
}

function round(seasonNumber: number, status: CurrentRound['status'], note: string): CurrentRound {
  return {
    roundId: `round-${seasonNumber}`, mediaId: 'series-1', seasonNumber, roundNumber: 1,
    status, rating: null, note, viewingMethod: null, startedAt: null, watchedAt: null, participantIds: [],
    version: 2, profileVersion: 9,
  }
}

it('switches progress and private records together while retaining each season cache', async () => {
  const progressRequests = new Map<number, number>()
  server.use(
    http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json(tvDetails)),
    http.get('*/api/v1/tmdb/tv/1399/season/:season', ({ params }) => HttpResponse.json({
      id: Number(params.season) + 10,
      name: `第 ${params.season} 季`, overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: Number(params.season),
      episodes: [{
        id: Number(params.season) * 100 + 1, name: `第 ${params.season} 季第一集`, overview: '', airDate: '2026-01-01',
        seasonNumber: Number(params.season), episodeNumber: 1, runtime: 45, stillPath: '',
      }],
    })),
    http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      return HttpResponse.json(seasonNumber === 1 ? round(1, 'watching', '第一季笔记') : round(2, 'none', '第二季笔记'))
    }),
    http.get('*/api/v1/records/series-1/progress', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      progressRequests.set(seasonNumber, (progressRequests.get(seasonNumber) ?? 0) + 1)
      return HttpResponse.json({
        roundId: `round-${seasonNumber}`, mediaId: 'series-1', seasonNumber,
        status: seasonNumber === 1 ? 'watching' : 'none', version: 2,
        watchedEpisodes: 0, totalEpisodes: 1, lastWatched: null, nextEpisode: null, episodes: [],
      })
    }),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<SeasonRecordWorkspace mediaId="series-1" tmdbId={1399} participants={[]} />)

  expect(await screen.findByRole('heading', { name: '第 1 季个人记录' })).toBeVisible()
  expect(screen.getByLabelText('私人笔记')).toHaveValue('第一季笔记')
  expect(await screen.findByText('第 1 季第一集')).toBeVisible()

  await user.selectOptions(screen.getByRole('combobox', { name: '选择季' }), '2')
  expect(await screen.findByRole('heading', { name: '第 2 季个人记录' })).toBeVisible()
  expect(screen.getByLabelText('私人笔记')).toHaveValue('第二季笔记')
  expect(screen.getByText('第 2 季第一集')).toBeVisible()

  await user.selectOptions(screen.getByRole('combobox', { name: '选择季' }), '1')
  expect(await screen.findByText('第 1 季第一集')).toBeVisible()
  expect(progressRequests.get(1)).toBe(1)
  expect(progressRequests.get(2)).toBe(1)
})

it('reuses the active season catalog title in archived round details', async () => {
  server.use(
    http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json(tvDetails)),
    http.get('*/api/v1/tmdb/tv/1399/season/:season', ({ params }) => HttpResponse.json({
      id: Number(params.season) + 10,
      name: `第 ${params.season} 季`, overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: Number(params.season),
      episodes: [{
        id: Number(params.season) * 100 + 1, name: `第 ${params.season} 季第一集`, overview: '', airDate: '2026-01-01',
        seasonNumber: Number(params.season), episodeNumber: 1, runtime: 45, stillPath: '',
      }],
    })),
    http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      return HttpResponse.json(seasonNumber === 1 ? round(1, 'watching', '本轮笔记') : round(2, 'none', ''))
    }),
    http.get('*/api/v1/records/series-1/progress', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      return HttpResponse.json({
        roundId: `round-${seasonNumber}`, mediaId: 'series-1', seasonNumber,
        status: seasonNumber === 1 ? 'watching' : 'none', version: 2,
        watchedEpisodes: 0, totalEpisodes: 1, lastWatched: null, nextEpisode: null, episodes: [],
      })
    }),
    http.get('*/api/v1/records/series-1/rounds/archived-1', () => HttpResponse.json({
      round: {
        roundId: 'archived-1', mediaId: 'series-1', seasonNumber: 1, roundNumber: 1,
        status: 'completed', rating: 9, note: '上一轮笔记', viewingMethod: null,
        startedAt: null, watchedAt: '2026-07-13T12:30:45Z', archivedAt: '2026-07-14T12:00:00Z',
      },
      episodes: [{
        id: 'episode-101', sourceId: '101', seasonId: 'season-1', seasonNumber: 1,
        episodeNumber: 1, absoluteNumber: 1, name: '', watched: true,
        startedAt: null, watchedAt: '2026-07-12T11:10:12Z',
      }],
    })),
    http.get('*/api/v1/records/series-1/rounds', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      return HttpResponse.json({ rounds: seasonNumber === 1 ? [{
        roundId: 'archived-1', mediaId: 'series-1', seasonNumber: 1,
        roundNumber: 1, startedAt: null, watchedAt: '2026-07-13T12:30:45Z', rating: 9,
      }] : [] })
    }),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<SeasonRecordWorkspace mediaId="series-1" tmdbId={1399} participants={[]} />)

  expect(await screen.findByText('第 1 季第一集')).toBeVisible()
  await user.click(await screen.findByRole('button', { name: '查看第 1 刷' }))

  const dialog = await screen.findByRole('dialog', { name: '第 1 刷记录' })
  expect(within(dialog).getByText('第 1 季第一集')).toBeVisible()
  expect(dialog).not.toHaveTextContent('未命名')
})
