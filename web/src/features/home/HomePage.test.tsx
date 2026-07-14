import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { HomePage } from './HomePage'

describe('HomePage', () => {
  it('shows continuing titles and recently updated private records', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
    const episodes = [
      { id: 'episode-1', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 1, absoluteNumber: 1, name: '第一集', watched: true, watchedAt: '2026-07-12T12:00:00Z' },
      { id: 'episode-2', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2, name: '第二集', watched: false, watchedAt: null },
      { id: 'episode-3', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 3, absoluteNumber: 3, name: '第三集', watched: false, watchedAt: null },
    ]
    let progressVersion = 1
    const progressBodies: unknown[] = []
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const status = new URL(request.url).searchParams.get('status')
        const continuing = [{
          id: 'series-1', source: 'local', mediaType: 'tv', title: '漫长的季节',
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
        mediaId: 'series-1', status: 'watching', version: progressVersion,
        watchedEpisodes: progressVersion === 1 ? 1 : 2, totalEpisodes: 3,
        lastWatched: progressVersion === 1 ? episodes[0] : { ...episodes[1], watched: true, watchedAt: '2026-07-13T12:00:00Z' },
        nextEpisode: progressVersion === 1 ? episodes[1] : episodes[2],
        episodes,
      })),
      http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        const body = await request.json() as { action: string }
        progressBodies.push(body)
        if (body.action === 'next') {
          progressVersion = 2
          return HttpResponse.json({
            mediaId: 'series-1', status: 'watching', version: 2, watchedEpisodes: 2, totalEpisodes: 3,
            lastWatched: { ...episodes[1], watched: true, watchedAt: '2026-07-13T12:00:00Z' },
            nextEpisode: episodes[2], episodes,
          })
        }
        progressVersion = 3
        return HttpResponse.json({
          mediaId: 'series-1', status: 'watching', version: 3, watchedEpisodes: 1, totalEpisodes: 3,
          lastWatched: episodes[0], nextEpisode: episodes[1], episodes,
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
    await waitFor(() => expect(progressBodies[0]).toMatchObject({ action: 'next', expectedVersion: 1 }))
    const undo = await screen.findByRole('button', { name: '撤销 漫长的季节 S01E02' })
    await user.click(undo)
    await waitFor(() => expect(progressBodies[1]).toMatchObject({ action: 'undo', episodeId: 'episode-2', expectedVersion: 2 }))
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
