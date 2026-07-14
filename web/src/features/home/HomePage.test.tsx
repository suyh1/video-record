import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { HomePage } from './HomePage'

describe('HomePage', () => {
  it('loads the next episode only from the active current season round', async () => {
    const progressScopes: Array<string | null> = []
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const item = {
          id: 'series-1', tmdbId: 1399, source: 'local', mediaType: 'tv', title: '漫长的季节',
          originalTitle: 'The Long Season', year: '2023', posterPath: null, status: 'watching',
        }
        return HttpResponse.json({
          items: new URL(request.url).searchParams.get('status') === 'watching' ? [item] : [item],
          nextCursor: null,
        })
      }),
      http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json({
        id: 1399, name: '漫长的季节', originalName: 'The Long Season', firstAirDate: '2023-04-22',
        posterPath: '', backdropPath: '', overview: '', numberOfSeasons: 2, numberOfEpisodes: 4,
        episodeRuntime: [45], genres: ['剧情'],
        seasons: [
          { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2023-04-22', seasonNumber: 1, episodeCount: 2 },
          { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 2 },
        ],
      })),
      http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
        const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
        return HttpResponse.json({
          roundId: `round-${seasonNumber}`, mediaId: 'series-1', seasonNumber, roundNumber: 1,
          status: seasonNumber === 2 ? 'watching' : 'completed', rating: null, note: null,
          viewingMethod: null, watchedAt: null, version: 1, profileVersion: 1,
        })
      }),
      http.get('*/api/v1/records/series-1/progress', ({ request }) => {
        const scope = new URL(request.url).searchParams.get('seasonNumber')
        progressScopes.push(scope)
        if (scope !== '2') return HttpResponse.json({ code: 'invalid_round_scope' }, { status: 400 })
        return HttpResponse.json({
          roundId: 'round-2', mediaId: 'series-1', seasonNumber: 2, status: 'watching', version: 1,
          watchedEpisodes: 1, totalEpisodes: 2, lastWatched: null, nextEpisode: null,
          episodes: [{
            id: 'episode-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1,
            absoluteNumber: 3, name: '', watched: true, watchedAt: '2026-07-12T12:00:00Z',
          }],
        })
      }),
      http.get('*/api/v1/tmdb/tv/1399/season/2', () => HttpResponse.json({
        id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2,
        episodes: [
          { id: 201, name: '第三集', overview: '', airDate: '2026-07-01', seasonNumber: 2, episodeNumber: 1, runtime: 45, stillPath: '' },
          { id: 202, name: '第四集', overview: '', airDate: '2026-07-08', seasonNumber: 2, episodeNumber: 2, runtime: 45, stillPath: '' },
        ],
      })),
    )

    renderWithQueryClient(<MemoryRouter><HomePage /></MemoryRouter>)

    expect(await screen.findByRole('button', { name: '推进 漫长的季节 下一集 S02E02' })).toBeVisible()
    expect(progressScopes).toEqual(['2'])
  })

  it('shows continuing titles and recently updated private records', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
    const firstEpisode = {
      id: 'episode-101', sourceId: '101', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 1,
      absoluteNumber: 1, name: '', watched: true, watchedAt: '2026-07-12T12:00:00Z',
    }
    const secondEpisode = {
      id: 'episode-102', sourceId: '102', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 2,
      absoluteNumber: 2, name: '', watched: true, watchedAt: '2026-07-13T12:00:00Z',
    }
    let progressVersion = 1
    const progressBodies: unknown[] = []
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const status = new URL(request.url).searchParams.get('status')
        const continuing = [{
          id: 'series-1', tmdbId: 1399, source: 'local', mediaType: 'tv', title: '漫长的季节',
          originalTitle: 'The Long Season', year: '2023', posterPath: null, status: 'watching',
        }]
        const recent = [
          continuing[0],
          {
            id: 'movie-1', source: 'local', mediaType: 'movie', title: '花样年华',
            originalTitle: 'In the Mood for Love', year: '2000', posterPath: null, status: 'completed',
          },
        ]
        return HttpResponse.json({ items: status === 'watching' ? continuing : recent, nextCursor: null })
      }),
      http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json({
        roundId: 'round-1', mediaId: 'series-1', seasonNumber: 1,
        status: progressVersion === 1 ? 'watching' : 'completed', version: progressVersion,
        watchedEpisodes: progressVersion === 1 ? 1 : 2, totalEpisodes: 2,
        lastWatched: progressVersion === 1 ? firstEpisode : secondEpisode,
        nextEpisode: null,
        episodes: progressVersion === 1 ? [firstEpisode] : [firstEpisode, secondEpisode],
      })),
      http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json({
        id: 1399, name: '漫长的季节', originalName: 'The Long Season', firstAirDate: '2023-04-22',
        posterPath: '', backdropPath: '', overview: '', numberOfSeasons: 2, numberOfEpisodes: 4,
        episodeRuntime: [45], genres: ['剧情'],
        seasons: [
          { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2023-04-22', seasonNumber: 1, episodeCount: 2 },
          { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 2 },
        ],
      })),
      http.get('*/api/v1/tmdb/tv/1399/season/1', () => HttpResponse.json({
        id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2023-04-22', seasonNumber: 1,
        episodes: [
          { id: 101, name: '第一集', overview: '', airDate: '2023-04-22', seasonNumber: 1, episodeNumber: 1, runtime: 45, stillPath: '' },
          { id: 102, name: '第二集', overview: '', airDate: '2023-04-29', seasonNumber: 1, episodeNumber: 2, runtime: 45, stillPath: '' },
        ],
      })),
      http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
        const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
        return HttpResponse.json({
          roundId: `round-${seasonNumber}`, mediaId: 'series-1', seasonNumber, roundNumber: 1,
          status: seasonNumber === 1 ? 'watching' : 'none', rating: null, note: null,
          viewingMethod: null, watchedAt: null, version: 1, profileVersion: 1,
        })
      }),
      http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        const body = await request.json() as { action: string }
        progressBodies.push(body)
        if (body.action === 'next') {
          progressVersion = 2
          return HttpResponse.json({
            roundId: 'round-1', mediaId: 'series-1', seasonNumber: 1,
            status: 'completed', version: 2, watchedEpisodes: 2, totalEpisodes: 2,
            lastWatched: secondEpisode, nextEpisode: null, episodes: [firstEpisode, secondEpisode],
          })
        }
        progressVersion = 3
        return HttpResponse.json({
          roundId: 'round-1', mediaId: 'series-1', seasonNumber: 1,
          status: 'watching', version: 3, watchedEpisodes: 1, totalEpisodes: 2,
          lastWatched: firstEpisode, nextEpisode: null, episodes: [firstEpisode],
        })
      }),
    )

    renderWithQueryClient(<MemoryRouter><HomePage /></MemoryRouter>)
    const user = userEvent.setup()

    expect(await screen.findByRole('heading', { name: '继续观看' })).toBeVisible()
    expect(await screen.findByText('1 部剧集')).toBeVisible()
    expect(screen.getAllByText('The Long Season')).toHaveLength(2)
    expect(screen.getByRole('heading', { name: '最近记录' })).toBeVisible()
    expect(screen.getByText('In the Mood for Love')).toBeVisible()
    expect(screen.getByText('看过')).toBeVisible()

    await user.click(await screen.findByRole('button', { name: '推进 漫长的季节 下一集 S01E02' }))
    await waitFor(() => expect(progressBodies[0]).toMatchObject({
      action: 'next', expectedVersion: 1, totalEpisodes: 2,
      episodeRefs: [{ sourceId: '102', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2 }],
    }))
    const undo = await screen.findByRole('button', { name: '撤销 漫长的季节 S01E02' })
    await user.click(undo)
    await waitFor(() => expect(progressBodies[1]).toMatchObject({
      action: 'undo', expectedVersion: 2, totalEpisodes: 2,
      episodeRefs: [{ sourceId: '102', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2 }],
    }))
  })

  it('keeps the title usable without claiming completion when TMDB is unavailable', async () => {
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const item = {
          id: 'series-1', tmdbId: 1399, source: 'local', mediaType: 'tv', title: '漫长的季节',
          originalTitle: 'The Long Season', year: '2023', posterPath: null, status: 'watching',
        }
        return HttpResponse.json({
          items: new URL(request.url).searchParams.get('status') === 'watching' ? [item] : [item],
          nextCursor: null,
        })
      }),
      http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json({
        mediaId: 'series-1', status: 'watching', version: 1, watchedEpisodes: 1, totalEpisodes: 1,
        lastWatched: {
          id: 'episode-101', sourceId: '101', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 1,
          absoluteNumber: 1, name: '', watched: true, watchedAt: '2026-07-12T12:00:00Z',
        },
        nextEpisode: null,
        episodes: [],
      })),
      http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json({ code: 'tmdb_unavailable' }, { status: 502 })),
    )

    renderWithQueryClient(<MemoryRouter><HomePage /></MemoryRouter>)

    expect(await screen.findAllByText('The Long Season')).toHaveLength(2)
    expect(await screen.findByRole('link', { name: '打开详情继续记录' })).toHaveAttribute('href', '/media/series-1')
    expect(screen.queryByText('已全部看完')).not.toBeInTheDocument()
  })

  it('offers search from an empty home and retries failed records', async () => {
    let attempts = 0
    server.use(http.get('*/api/v1/library', () => {
      attempts += 1
      if (attempts <= 2) return HttpResponse.json({ code: 'internal_error' }, { status: 500 })
      return HttpResponse.json({ items: [], nextCursor: null })
    }))
    const onSearch = vi.fn()
    const user = userEvent.setup()

    renderWithQueryClient(<MemoryRouter><HomePage onSearch={onSearch} /></MemoryRouter>)

    await user.click(await screen.findByRole('button', { name: '重新加载首页' }))
    await user.click(await screen.findByRole('button', { name: '搜索影视' }))

    expect(onSearch).toHaveBeenCalledOnce()
  })
})
