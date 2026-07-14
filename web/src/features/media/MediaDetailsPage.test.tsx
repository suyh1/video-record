import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { MediaDetailsPage } from './MediaDetailsPage'

function renderDetails(mediaID: string) {
  return renderWithQueryClient(
    <MemoryRouter initialEntries={[`/media/${mediaID}`]}>
      <Routes><Route path="/media/:mediaId" element={<MediaDetailsPage />} /></Routes>
    </MemoryRouter>,
  )
}

it('uses movie rounds, profile versions, and no flat watch-event history', async () => {
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  let eventRequests = 0
  let tagsVersion = ''
  server.use(
    http.get('*/api/v1/media/movie-1', () => HttpResponse.json({
      id: 'movie-1', tmdbId: null, mediaType: 'movie', title: '花样年华', originalTitle: 'In the Mood for Love',
      releaseDate: '2000-09-29', overview: '两位邻居在克制与靠近之间建立起一段关系。',
      externalTitle: '', externalOverview: '', posterPath: null, backdropPath: '', runtimeMinutes: 98, genres: ['剧情'],
    })),
    http.get('*/api/v1/records/movie-1', () => HttpResponse.json({
      mediaId: 'movie-1', status: 'completed', rating: 9.4, note: null,
      watchedAt: '2026-07-12T20:30:45Z', viewingMethod: '影院', version: 12,
    })),
    http.get('*/api/v1/records/movie-1/rounds/current', () => HttpResponse.json({
      roundId: 'movie-round-1', mediaId: 'movie-1', seasonNumber: null, roundNumber: 1,
      status: 'completed', rating: 9.4, note: '雨夜与走廊。', viewingMethod: '影院',
      watchedAt: '2026-07-12T20:30:45Z', participantIds: [], version: 4, profileVersion: 12,
    })),
    http.get('*/api/v1/records/movie-1/rounds', () => HttpResponse.json({ rounds: [] })),
    http.get('*/api/v1/records/movie-1/events', () => {
      eventRequests += 1
      return HttpResponse.json([])
    }),
    http.get('*/api/v1/household/participants', () => HttpResponse.json([])),
    http.get('*/api/v1/records/movie-1/tags', () => HttpResponse.json({ tags: ['怀旧'] })),
    http.put('*/api/v1/records/movie-1/tags', ({ request }) => {
      tagsVersion = request.headers.get('If-Match') ?? ''
      return new HttpResponse(null, { status: 204, headers: { ETag: '"13"' } })
    }),
    http.get('*/api/v1/collections', () => HttpResponse.json([])),
    http.get('*/api/v1/household/records/movie-1/sharing', () => HttpResponse.json({
      mediaId: 'movie-1', shareRating: false, shareReview: false, sharedReview: null, version: 12,
    })),
  )
  const user = userEvent.setup()
  renderDetails('movie-1')

  expect(await screen.findByRole('heading', { name: '花样年华', level: 1 })).toBeVisible()
  expect(await screen.findByRole('heading', { name: '个人记录' })).toBeVisible()
  expect(screen.getByLabelText('私人笔记')).toHaveValue('雨夜与走廊。')
  expect(screen.getByRole('heading', { name: '多刷' })).toBeVisible()
  expect(screen.queryByText('观看历史')).not.toBeInTheDocument()
  expect(eventRequests).toBe(0)

  await user.click(screen.getByText('家庭与整理'))
  const tags = await screen.findByRole('textbox', { name: '私人标签' })
  await user.clear(tags)
  await user.type(tags, '经典')
  await user.click(screen.getByRole('button', { name: '保存标签' }))
  await waitFor(() => expect(tagsVersion).toBe('"12"'))
})

it('orders a TV season workspace as selector, episodes, private record, and rewatch archive', async () => {
  let eventRequests = 0
  server.use(
    http.get('*/api/v1/media/series-1', () => HttpResponse.json({
      id: 'series-1', tmdbId: 1399, mediaType: 'tv', title: '漫长的季节', originalTitle: 'The Long Season',
      releaseDate: '2023-04-22', overview: '北方小城的旧案。', externalTitle: '', externalOverview: '',
      posterPath: null, backdropPath: '', runtimeMinutes: 45, genres: ['剧情'],
    })),
    http.get('*/api/v1/records/series-1', () => HttpResponse.json({
      mediaId: 'series-1', status: 'watching', rating: null, note: null,
      watchedAt: null, viewingMethod: null, version: 7,
    })),
    http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json({
      id: 1399, name: '漫长的季节', originalName: 'The Long Season', firstAirDate: '2023-04-22',
      posterPath: '', backdropPath: '', overview: '北方小城的旧案。', numberOfSeasons: 2, numberOfEpisodes: 2,
      episodeRuntime: [45], genres: ['剧情'],
      seasons: [
        { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2023-04-22', seasonNumber: 1, episodeCount: 1 },
        { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 1 },
      ],
    })),
    http.get('*/api/v1/tmdb/tv/1399/credits', () => HttpResponse.json({ cast: [] })),
    http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      return HttpResponse.json({
        roundId: `season-round-${seasonNumber}`, mediaId: 'series-1', seasonNumber, roundNumber: 1,
        status: seasonNumber === 1 ? 'watching' : 'none', rating: null,
        note: seasonNumber === 1 ? '第一季私人笔记' : null, viewingMethod: null,
        watchedAt: null, participantIds: [], version: 2, profileVersion: 7,
      })
    }),
    http.get('*/api/v1/records/series-1/progress', ({ request }) => {
      const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
      return HttpResponse.json({
        roundId: `season-round-${seasonNumber}`, mediaId: 'series-1', seasonNumber,
        status: seasonNumber === 1 ? 'watching' : 'none', version: 2,
        watchedEpisodes: 0, totalEpisodes: 1, lastWatched: null, nextEpisode: null, episodes: [],
      })
    }),
    http.get('*/api/v1/tmdb/tv/1399/season/:season', ({ params }) => HttpResponse.json({
      id: Number(params.season) + 10, name: `第 ${params.season} 季`, overview: '', posterPath: '',
      airDate: '2023-04-22', seasonNumber: Number(params.season),
      episodes: [{
        id: Number(params.season) * 100 + 1, name: `第 ${params.season} 季第一集`, overview: '',
        airDate: '2023-04-22', seasonNumber: Number(params.season), episodeNumber: 1, runtime: 45, stillPath: '',
      }],
    })),
    http.get('*/api/v1/records/series-1/rounds', () => HttpResponse.json({ rounds: [] })),
    http.get('*/api/v1/records/series-1/events', () => {
      eventRequests += 1
      return HttpResponse.json([])
    }),
    http.get('*/api/v1/household/participants', () => HttpResponse.json([])),
  )
  renderDetails('series-1')

  const selector = await screen.findByRole('combobox', { name: '选择季' })
  const episodes = await screen.findByRole('heading', { name: '剧集进度' })
  const privateRecord = await screen.findByRole('heading', { name: '第 1 季个人记录' })
  const rewatch = await screen.findByRole('heading', { name: '多刷' })

  expect(selector.compareDocumentPosition(episodes) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(episodes.compareDocumentPosition(privateRecord) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(privateRecord.compareDocumentPosition(rewatch) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(screen.queryByText('观看历史')).not.toBeInTheDocument()
  expect(eventRequests).toBe(0)
})
