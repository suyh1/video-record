import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'

import { server } from '../test/server'
import { createMediaFromTMDB, getTMDBCredits, getTMDBSeason, getTMDBTV } from './client'

describe('API client protected writes', () => {
  beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

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
    expect(credits[0]?.character).toBe('艾德·史塔克')
  })
})
