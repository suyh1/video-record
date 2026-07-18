import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { TMDBPreviewPage } from './TMDBPreviewPage'

const signature = 'd'.repeat(64)
const posterURL = `/api/v1/public/tmdb/images/w342/wild-dog.jpg?expires=1784200000&signature=${signature}`
const backdropURL = `/api/v1/public/tmdb/images/w1280/wild-dog-bg.jpg?expires=1784200000&signature=${signature}`
const portraitURL = `/api/v1/public/tmdb/images/w300/cast.jpg?expires=1784200000&signature=${signature}`

function renderPreview(path: string) {
  return renderWithQueryClient(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/tmdb/:mediaType/:tmdbId" element={<TMDBPreviewPage />} />
        <Route path="/media/:mediaId" element={<p data-testid="local-detail">本地详情</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('TMDBPreviewPage', () => {
  it('renders a signed-image TV preview using GET requests only', async () => {
    let localRequests = 0
    let writeRequests = 0
    server.use(
      http.get('*/api/v1/tmdb/tv/12345', () => HttpResponse.json({
        id: 12345,
        name: '野狗骨头',
        originalName: 'Wild Dog Bones',
        firstAirDate: '2026-01-01',
        posterPath: posterURL,
        backdropPath: backdropURL,
        overview: '两个家庭跨越多年的命运纠葛。',
        numberOfSeasons: 1,
        numberOfEpisodes: 12,
        episodeRuntime: [45],
        genres: ['剧情'],
        seasons: [],
      })),
      http.get('*/api/v1/tmdb/tv/12345/credits', () => HttpResponse.json({
        cast: [{ id: 7, name: '宋佳', character: '陈异', profilePath: portraitURL, order: 0 }],
      })),
      http.get('*/api/v1/media/:mediaId', () => {
        localRequests += 1
        return HttpResponse.json({ code: 'not_found' }, { status: 404 })
      }),
      http.get('*/api/v1/records/:mediaId', () => {
        localRequests += 1
        return HttpResponse.json({ code: 'not_found' }, { status: 404 })
      }),
      http.post('*/api/v1/media/tmdb/:mediaType/:externalId', () => {
        writeRequests += 1
        return HttpResponse.json({ code: 'unexpected_write' }, { status: 500 })
      }),
    )

    renderPreview('/tmdb/tv/12345')

    expect(await screen.findByRole('heading', { level: 1, name: '野狗骨头' })).toBeVisible()
    expect(screen.getByText('TMDB 预览')).toBeVisible()
    expect(screen.getByText('两个家庭跨越多年的命运纠葛。')).toBeVisible()
    expect(screen.getByRole('img', { name: '野狗骨头 海报' })).toHaveAttribute('src', posterURL)
    expect(screen.getByRole('img', { name: '宋佳 饰 陈异' })).toHaveAttribute('src', portraitURL)
    expect(screen.getByRole('button', { name: '开始记录' })).toBeEnabled()
    expect(document.querySelector('.media-atmosphere-page')).not.toBeNull()
    expect(localRequests).toBe(0)
    expect(writeRequests).toBe(0)
  })

  it('rejects invalid preview parameters without requesting TMDB', async () => {
    let tmdbRequests = 0
    server.use(http.get('*/api/v1/tmdb/:mediaType/:tmdbId', () => {
      tmdbRequests += 1
      return HttpResponse.json({})
    }))

    renderPreview('/tmdb/person/not-a-number')

    expect(await screen.findByRole('alert')).toHaveTextContent('无法打开 TMDB 详情')
    expect(tmdbRequests).toBe(0)
  })

  it('keeps a retryable error when TMDB details fail', async () => {
    let detailRequests = 0
    server.use(
      http.get('*/api/v1/tmdb/movie/329865', () => {
        detailRequests += 1
        return detailRequests === 1
          ? HttpResponse.json({ code: 'tmdb_unavailable' }, { status: 503 })
          : HttpResponse.json({
              id: 329865,
              title: '降临',
              originalTitle: 'Arrival',
              releaseDate: '2016-11-10',
              posterPath: posterURL,
              backdropPath: backdropURL,
              overview: '语言学家尝试理解陌生来客。',
              runtime: 116,
              genres: ['科幻'],
            })
      }),
      http.get('*/api/v1/tmdb/movie/329865/credits', () => HttpResponse.json({ cast: [] })),
    )
    const user = userEvent.setup()
    renderPreview('/tmdb/movie/329865')

    expect(await screen.findByRole('alert')).toHaveTextContent('无法获取 TMDB 详情')
    await user.click(screen.getByRole('button', { name: '重新加载详情' }))

    await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: '降临' })).toBeVisible())
    expect(detailRequests).toBe(2)
  })

  it('imports a movie and saves its selected status only after explicit submission', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'preview-csrf')
    const requests: string[] = []
    server.use(
      http.get('*/api/v1/tmdb/movie/329865', () => HttpResponse.json({
        id: 329865,
        title: '降临',
        originalTitle: 'Arrival',
        releaseDate: '2016-11-10',
        posterPath: posterURL,
        backdropPath: backdropURL,
        overview: '语言学家尝试理解陌生来客。',
        runtime: 116,
        genres: ['科幻'],
      })),
      http.get('*/api/v1/tmdb/movie/329865/credits', () => HttpResponse.json({ cast: [] })),
      http.post('*/api/v1/media/tmdb/movie/329865', () => {
        requests.push('import')
        return HttpResponse.json({
          id: 'local-arrival', tmdbId: 329865, mediaType: 'movie', title: '降临',
          externalTitle: '降临', externalOverview: '语言学家尝试理解陌生来客。',
          originalTitle: 'Arrival', releaseDate: '2016-11-10', overview: '语言学家尝试理解陌生来客。',
          posterPath: posterURL, backdropPath: backdropURL, runtimeMinutes: 116, genres: ['科幻'],
        })
      }),
      http.put('*/api/v1/records/local-arrival/rounds/current', async ({ request }) => {
        requests.push('record')
        expect(request.headers.get('If-Match')).toBe('"0"')
        expect(await request.json()).toEqual({ status: 'wishlist' })
        return HttpResponse.json({
          roundId: 'round-arrival', mediaId: 'local-arrival', seasonNumber: null, roundNumber: 1,
          status: 'wishlist', rating: null, note: null, viewingMethod: null, startedAt: null, watchedAt: null,
          participantIds: [], version: 1, profileVersion: 1,
        })
      }),
    )
    const user = userEvent.setup()
    renderPreview('/tmdb/movie/329865')

    await screen.findByRole('heading', { level: 1, name: '降临' })
    expect(requests).toEqual([])
    expect(screen.getByRole('button', { name: '保存记录' })).toBeDisabled()

    await user.click(screen.getByRole('radio', { name: '想看' }))
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    expect(await screen.findByTestId('local-detail')).toBeVisible()
    expect(requests).toEqual(['import', 'record'])
  })

  it('imports a TV item only after the user starts recording', async () => {
    let importRequests = 0
    server.use(
      http.get('*/api/v1/tmdb/tv/12345', () => HttpResponse.json({
        id: 12345, name: '野狗骨头', originalName: 'Wild Dog Bones', firstAirDate: '2026-01-01',
        posterPath: posterURL, backdropPath: backdropURL, overview: '两个家庭跨越多年的命运纠葛。',
        numberOfSeasons: 1, numberOfEpisodes: 12, episodeRuntime: [45], genres: ['剧情'], seasons: [],
      })),
      http.get('*/api/v1/tmdb/tv/12345/credits', () => HttpResponse.json({ cast: [] })),
      http.post('*/api/v1/media/tmdb/tv/12345', () => {
        importRequests += 1
        return HttpResponse.json({
          id: 'local-wild-dog', tmdbId: 12345, mediaType: 'tv', title: '野狗骨头',
          externalTitle: '野狗骨头', externalOverview: '', originalTitle: 'Wild Dog Bones',
          releaseDate: '2026-01-01', overview: '', posterPath: posterURL, backdropPath: backdropURL,
          runtimeMinutes: 45, genres: ['剧情'],
        })
      }),
    )
    const user = userEvent.setup()
    renderPreview('/tmdb/tv/12345')

    const start = await screen.findByRole('button', { name: '开始记录' })
    expect(importRequests).toBe(0)
    await user.click(start)

    expect(await screen.findByTestId('local-detail')).toBeVisible()
    expect(importRequests).toBe(1)
  })

  it('preserves a selected movie status when importing fails', async () => {
    server.use(
      http.get('*/api/v1/tmdb/movie/329865', () => HttpResponse.json({
        id: 329865, title: '降临', originalTitle: 'Arrival', releaseDate: '2016-11-10',
        posterPath: posterURL, backdropPath: backdropURL, overview: '', runtime: 116, genres: ['科幻'],
      })),
      http.get('*/api/v1/tmdb/movie/329865/credits', () => HttpResponse.json({ cast: [] })),
      http.post('*/api/v1/media/tmdb/movie/329865', () => HttpResponse.json(
        { code: 'tmdb_unavailable' },
        { status: 503 },
      )),
    )
    const user = userEvent.setup()
    renderPreview('/tmdb/movie/329865')

    await screen.findByRole('heading', { level: 1, name: '降临' })
    const watching = screen.getByRole('radio', { name: '在看' })
    await user.click(watching)
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
    expect(watching).toHaveAttribute('aria-checked', 'true')
    expect(screen.queryByTestId('local-detail')).not.toBeInTheDocument()
  })
})
