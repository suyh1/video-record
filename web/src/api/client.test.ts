import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'

import { server } from '../test/server'
import { createMediaFromTMDB } from './client'

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
})
