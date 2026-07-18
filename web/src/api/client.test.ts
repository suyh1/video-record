import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { server } from '../test/server'
import {
  APIError,
  createMediaFromTMDB,
  getCurrentRound,
  getEpisodeProgress,
  getRoundDetail,
  getRoundHistory,
  getTMDBCredits,
  getTMDBHighlights,
  getTMDBSeason,
  getTMDBTV,
  logoutUser,
  startRewatch,
  updateCurrentRound,
  updateEpisodeProgress,
} from './client'

describe('public TMDB highlights client', () => {
  it('requests the same-origin public endpoint and returns its items unchanged', async () => {
    const backdropURL = '/api/v1/public/tmdb/images/w1280/arrival.jpg?expires=42&signature=signed'
    const items = [
      {
        id: 329865,
        mediaType: 'movie' as const,
        title: '降临',
        originalTitle: 'Arrival',
        year: '2016',
        overview: '语言学家试图理解外星来客。',
        backdropURL,
      },
      {
        id: 1399,
        mediaType: 'tv' as const,
        title: '权力的游戏',
        originalTitle: 'Game of Thrones',
        year: '2011',
        overview: '凛冬将至。',
        backdropURL: '/api/v1/public/tmdb/images/w1280/winter.jpg?expires=42&signature=signed',
      },
    ]
    server.use(
      http.get('*/api/v1/public/tmdb/highlights', ({ request }) => {
        expect(new URL(request.url).pathname).toBe('/api/v1/public/tmdb/highlights')
        expect(new URL(request.url).origin).toBe(window.location.origin)
        return HttpResponse.json({ items })
      }),
    )

    const result = await getTMDBHighlights()

    expect(result).toEqual(items)
    expect(result[0]?.backdropURL).toBe(backdropURL)
  })

  it('forwards an AbortSignal to the public request', async () => {
    const controller = new AbortController()
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ items: [] }), {
        headers: { 'Content-Type': 'application/json' },
        status: 200,
      }),
    )

    try {
      await getTMDBHighlights(controller.signal)

      expect(fetchSpy).toHaveBeenCalledWith(
        new URL('/api/v1/public/tmdb/highlights', window.location.origin),
        expect.objectContaining({ signal: controller.signal }),
      )
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('preserves API errors instead of synthesizing fallback highlights', async () => {
    server.use(
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json(
        { code: 'tmdb_unavailable', requestId: 'request-highlights' },
        { status: 503 },
      )),
    )

    const request = getTMDBHighlights()

    await expect(request).rejects.toBeInstanceOf(APIError)
    await expect(request).rejects.toMatchObject({
      code: 'tmdb_unavailable',
      requestId: 'request-highlights',
      status: 503,
    })
  })
})

describe('API client protected writes', () => {
  beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

  it('logs out without depending on tab CSRF state', async () => {
    sessionStorage.removeItem('video-record.csrf-token')
    server.use(
      http.post('*/api/v1/auth/logout', ({ request }) => {
        expect(request.headers.has('X-CSRF-Token')).toBe(false)
        return new HttpResponse(null, { status: 204 })
      }),
    )

    await logoutUser()
  })

  it('sends an idempotency key when creating media from TMDB', async () => {
    server.use(
      http.post('*/api/v1/media/tmdb/movie/329865', ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        return HttpResponse.json({
          id: 'media-1',
          mediaType: 'movie',
          title: 'Arrival',
          originalTitle: 'Arrival',
          year: '2016',
          overview: '',
          posterPath: null,
        })
      }),
    )

    await createMediaFromTMDB({
      id: 'tmdb-movie-329865',
      externalId: 329865,
      source: 'tmdb',
      mediaType: 'movie',
      title: 'Arrival',
      originalTitle: 'Arrival',
      year: '2016',
      posterPath: null,
      status: 'none',
    })
  })

  it('reads live TV, season, and credits through authenticated server routes', async () => {
    server.use(
      http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json({
        id: 1399, name: '权力的游戏', originalName: 'Game of Thrones', firstAirDate: '2011-04-17',
        posterPath: '', backdropPath: '/backdrop.jpg', overview: '冬天将至', numberOfSeasons: 8,
        numberOfEpisodes: 73, episodeRuntime: [57], genres: ['剧情'],
        seasons: [{ id: 3624, name: '第 1 季', overview: '', posterPath: '', airDate: '2011-04-17', seasonNumber: 1, episodeCount: 10 }],
      })),
      http.get('*/api/v1/tmdb/tv/1399/season/1', () => HttpResponse.json({
        id: 3624, name: '第 1 季', overview: '', posterPath: '', airDate: '2011-04-17', seasonNumber: 1,
        episodes: [{ id: 63056, name: '凛冬将至', overview: '', airDate: '2011-04-17', seasonNumber: 1, episodeNumber: 1, runtime: 62, stillPath: '/winter.jpg' }],
      })),
      http.get('*/api/v1/tmdb/tv/1399/credits', () => HttpResponse.json({
        cast: [{ id: 1, name: '肖恩·宾', character: '艾德·史塔克', profilePath: '/sean.jpg', order: 0 }],
      })),
    )
    const controller = new AbortController()

    const tv = await getTMDBTV(1399, controller.signal)
    const season = await getTMDBSeason(1399, 1, controller.signal)
    const credits = await getTMDBCredits('tv', 1399, controller.signal)

    expect(tv.seasons[0]?.episodeCount).toBe(10)
    expect(season.episodes[0]?.stillPath).toBe('/winter.jpg')
    expect(credits.cast[0]?.character).toBe('艾德·史塔克')
  })

  it('scopes movie and season round reads without ambiguous URLs', async () => {
    server.use(
      http.get('*/api/v1/records/movie-1/rounds/current', ({ request }) => {
        expect(new URL(request.url).search).toBe('')
        return HttpResponse.json({ roundId: '', mediaId: 'movie-1', seasonNumber: null, roundNumber: 1, status: 'none', rating: null, note: null, viewingMethod: null, watchedAt: null, version: 0, profileVersion: 0 })
      }),
      http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
        return HttpResponse.json({ roundId: 'round-2', mediaId: 'series-1', seasonNumber: 2, roundNumber: 1, status: 'watching', rating: null, note: null, viewingMethod: null, watchedAt: null, version: 1, profileVersion: 1 })
      }),
    )

    expect((await getCurrentRound('movie-1')).seasonNumber).toBeNull()
    expect((await getCurrentRound('series-1', 2)).seasonNumber).toBe(2)
  })

  it('writes rounds and progress with versions, season scope, CSRF, and idempotency', async () => {
    server.use(
      http.put('*/api/v1/records/series-1/rounds/current', async ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
        expect(request.headers.get('If-Match')).toBe('"3"')
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        expect(await request.json()).toMatchObject({ status: 'completed' })
        return HttpResponse.json({ roundId: 'round-2', mediaId: 'series-1', seasonNumber: 2, roundNumber: 1, status: 'completed', rating: null, note: null, viewingMethod: null, watchedAt: '2026-07-13T12:00:01Z', version: 4, profileVersion: 1 })
      }),
      http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        expect(await request.json()).toMatchObject({ action: 'set_time', expectedVersion: 4 })
        return HttpResponse.json({ roundId: 'round-2', mediaId: 'series-1', seasonNumber: 2, status: 'watching', version: 5, watchedEpisodes: 1, totalEpisodes: 2, lastWatched: null, nextEpisode: null, episodes: [] })
      }),
    )

    await updateCurrentRound('series-1', 2, 3, { status: 'completed', watchedAt: '2026-07-13T12:00:01Z' })
    await updateEpisodeProgress('series-1', 2, { action: 'set_time', expectedVersion: 4, episodeId: 'episode-1', watchedAt: '2026-07-13T12:00:01Z' })
  })

  it('reads round history/detail and replays the scoped rewatch command', async () => {
    server.use(
      http.get('*/api/v1/records/series-1/rounds', ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('1')
        return HttpResponse.json({ rounds: [{ roundId: 'archived-1', mediaId: 'series-1', seasonNumber: 1, roundNumber: 1, watchedAt: '2026-07-13T12:00:01Z', rating: 9 }] })
      }),
      http.get('*/api/v1/records/series-1/rounds/archived-1', ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('1')
        return HttpResponse.json({ round: { roundId: 'archived-1', mediaId: 'series-1', seasonNumber: 1, roundNumber: 1, status: 'completed', rating: 9, note: '第一轮', viewingMethod: null, watchedAt: '2026-07-13T12:00:01Z', archivedAt: '2026-07-14T12:00:01Z' }, episodes: [] })
      }),
      http.post('*/api/v1/records/series-1/rounds/current/rewatch', ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('1')
        expect(request.headers.get('If-Match')).toBe('"2"')
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        return HttpResponse.json({ archived: {}, current: { roundId: 'current-2', version: 1 } })
      }),
      http.get('*/api/v1/records/series-1/progress', ({ request }) => {
        expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('1')
        return HttpResponse.json({ roundId: 'current-2', mediaId: 'series-1', seasonNumber: 1, status: 'watching', version: 1, watchedEpisodes: 0, totalEpisodes: 0, lastWatched: null, nextEpisode: null, episodes: [] })
      }),
    )

    expect(await getRoundHistory('series-1', 1)).toHaveLength(1)
    expect((await getRoundDetail('series-1', 'archived-1', 1)).round.note).toBe('第一轮')
    expect((await startRewatch('series-1', 1, 2)).current.roundId).toBe('current-2')
    expect((await getEpisodeProgress('series-1', 1)).seasonNumber).toBe(1)
  })
})
