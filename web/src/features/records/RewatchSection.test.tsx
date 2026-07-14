import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useState } from 'react'
import { beforeEach, expect, it } from 'vitest'

import type { CurrentRound } from '../../api/types'
import { formatLocalSeconds } from '../../lib/dateTime'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { RewatchSection } from './RewatchSection'

const completedMovie: CurrentRound = {
  roundId: 'movie-round-1', mediaId: 'movie-1', seasonNumber: null, roundNumber: 1,
  status: 'completed', rating: 9.2, note: '第一刷笔记', viewingMethod: '影院',
  watchedAt: '2026-07-13T12:30:45Z', version: 4, profileVersion: 8,
}

beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

it('shows an empty archive and disables rewatch until the current round is complete', async () => {
  server.use(http.get('*/api/v1/records/movie-1/rounds', () => HttpResponse.json({ rounds: [] })))
  renderWithQueryClient(
    <RewatchSection round={{ ...completedMovie, status: 'watching', watchedAt: null }} />,
  )

  expect(await screen.findByText('暂无多刷记录')).toBeVisible()
  expect(screen.getByRole('button', { name: '再刷' })).toBeDisabled()
  expect(screen.getByText('当前一刷完成后可再刷')).toBeVisible()
})

it('atomically replaces the current round and adds the archived summary', async () => {
  server.use(
    http.get('*/api/v1/records/movie-1/rounds', () => HttpResponse.json({ rounds: [] })),
    http.post('*/api/v1/records/movie-1/rounds/current/rewatch', ({ request }) => {
      expect(request.headers.get('If-Match')).toBe('"4"')
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      return HttpResponse.json({
        archived: { ...completedMovie, archivedAt: '2026-07-14T12:00:00Z' },
        current: {
          ...completedMovie,
          roundId: 'movie-round-2', roundNumber: 2, status: 'watching', rating: null,
          note: null, viewingMethod: null, watchedAt: null, version: 1,
        },
      })
    }),
  )
  const user = userEvent.setup()

  function Harness() {
    const [round, setRound] = useState(completedMovie)
    return <RewatchSection round={round} onRewatched={setRound} />
  }

  renderWithQueryClient(<Harness />)
  await user.click(await screen.findByRole('button', { name: '再刷' }))

  expect(await screen.findByText('第 1 刷')).toBeVisible()
  expect(screen.getByText(formatLocalSeconds(completedMovie.watchedAt!))).toBeVisible()
  expect(screen.getByRole('button', { name: '再刷' })).toBeDisabled()
})

it('leaves the current round and history unchanged when rewatch fails', async () => {
  server.use(
    http.get('*/api/v1/records/movie-1/rounds', () => HttpResponse.json({ rounds: [] })),
    http.post('*/api/v1/records/movie-1/rounds/current/rewatch', () =>
      HttpResponse.json({ code: 'internal_error' }, { status: 500 })),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<RewatchSection round={completedMovie} />)

  await user.click(await screen.findByRole('button', { name: '再刷' }))

  expect(await screen.findByRole('alert')).toHaveTextContent('再刷失败')
  expect(screen.getByRole('button', { name: '再刷' })).toBeEnabled()
  expect(screen.queryByText('第 1 刷')).not.toBeInTheDocument()
})

it('opens a private season archive with second-precision episode times', async () => {
  const seasonRound = { ...completedMovie, mediaId: 'series-1', seasonNumber: 2 }
  server.use(
    http.get('*/api/v1/records/series-1/rounds', ({ request }) => {
      expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
      return HttpResponse.json({ rounds: [{
        roundId: 'season-round-1', mediaId: 'series-1', seasonNumber: 2,
        roundNumber: 1, watchedAt: seasonRound.watchedAt, rating: 9.2,
      }] })
    }),
    http.get('*/api/v1/records/series-1/rounds/season-round-1', () => HttpResponse.json({
      round: { ...seasonRound, roundId: 'season-round-1', archivedAt: '2026-07-14T12:00:00Z' },
      episodes: [{
        id: 'episode-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2,
        episodeNumber: 1, absoluteNumber: 4, name: '重逢', watched: true,
        watchedAt: '2026-07-12T11:10:12Z',
      }],
    })),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<RewatchSection round={seasonRound} />)

  expect(await screen.findByText('9.2 / 10')).toBeVisible()
  await user.click(screen.getByRole('button', { name: '查看第 1 刷' }))

  const dialog = await screen.findByRole('dialog', { name: '第 1 刷记录' })
  expect(dialog).toHaveTextContent('第一刷笔记')
  expect(dialog).toHaveTextContent('影院')
  expect(dialog).toHaveTextContent('S02E01')
  expect(dialog).toHaveTextContent(formatLocalSeconds('2026-07-12T11:10:12Z'))
})

it('does not render episode details for a movie archive', async () => {
  server.use(
    http.get('*/api/v1/records/movie-1/rounds', () => HttpResponse.json({ rounds: [{
      roundId: 'movie-round-1', mediaId: 'movie-1', seasonNumber: null,
      roundNumber: 1, watchedAt: completedMovie.watchedAt, rating: 9.2,
    }] })),
    http.get('*/api/v1/records/movie-1/rounds/movie-round-1', () => HttpResponse.json({
      round: { ...completedMovie, archivedAt: '2026-07-14T12:00:00Z' },
      episodes: [],
    })),
  )
  const user = userEvent.setup()
  renderWithQueryClient(<RewatchSection round={completedMovie} />)

  await user.click(await screen.findByRole('button', { name: '查看第 1 刷' }))
  await waitFor(() => expect(screen.getByRole('dialog', { name: '第 1 刷记录' })).toBeVisible())
  expect(screen.queryByText(/S\d{2}E\d{2}/)).not.toBeInTheDocument()
})
