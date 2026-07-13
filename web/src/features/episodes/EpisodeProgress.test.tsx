import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, expect, it } from 'vitest'

import type { SeriesProgress } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { EpisodeProgress } from './EpisodeProgress'

const initialProgress: SeriesProgress = {
  mediaId: 'series-1',
  status: 'watching',
  version: 1,
  watchedEpisodes: 1,
  totalEpisodes: 6,
  lastWatched: null,
  nextEpisode: null,
  episodes: [
    { id: 's1e1', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 1, absoluteNumber: 1, name: '第一集', watched: true, watchedAt: '2026-07-12T12:00:00Z' },
    { id: 's1e2', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 2, absoluteNumber: 2, name: '第二集', watched: false, watchedAt: null },
    { id: 's1e3', seasonId: 'season-1', seasonNumber: 1, episodeNumber: 3, absoluteNumber: 3, name: '第三集', watched: false, watchedAt: null },
    { id: 's2e1', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1, absoluteNumber: 4, name: '第四集', watched: false, watchedAt: null },
    { id: 's2e2', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 2, absoluteNumber: 5, name: '第五集', watched: false, watchedAt: null },
    { id: 's2e3', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 3, absoluteNumber: 6, name: '第六集', watched: false, watchedAt: null },
  ],
}

beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

it('shows season episode labels together with absolute episode counts', async () => {
  server.use(http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json(initialProgress)))
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" />)

  const episode = await screen.findByRole('button', { name: '标记 S02E02 已看' })
  expect(episode).toHaveTextContent('S02E02')
  expect(episode).toHaveTextContent('全剧第 5 集')
  expect(screen.getByText('1 / 6 集')).toBeVisible()
})

it('advances exactly one next episode and offers an undo action', async () => {
  let savedBody: unknown
  server.use(
    http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json(initialProgress)),
    http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
      savedBody = await request.json()
      expect(request.headers.get('Idempotency-Key')).toBeTruthy()
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      return HttpResponse.json({
        ...initialProgress,
        version: 2,
        watchedEpisodes: 2,
        lastWatched: { ...initialProgress.episodes[1], watched: true, watchedAt: '2026-07-13T12:00:00Z' },
        nextEpisode: initialProgress.episodes[2],
        episodes: initialProgress.episodes.map((episode) => episode.id === 's1e2' ? { ...episode, watched: true, watchedAt: '2026-07-13T12:00:00Z' } : episode),
      })
    }),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" />)

  await user.click(await screen.findByRole('button', { name: '推进下一集 S01E02' }))

  await waitFor(() => expect(savedBody).toMatchObject({ action: 'next', expectedVersion: 1 }))
  expect(await screen.findByRole('status')).toHaveTextContent('已推进至 S01E02')
  expect(screen.getByRole('button', { name: '撤销 S01E02' })).toBeVisible()
})
