import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, expect, it } from 'vitest'

import type { EpisodeProgressItem, SeriesProgress } from '../../api/types'
import { formatLocalSeconds } from '../../lib/dateTime'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { EpisodeProgress } from './EpisodeProgress'

const now = new Date(2026, 6, 14, 17, 8, 9)
const watchedAt = '2026-07-13T12:00:01Z'

const watchedEpisode: EpisodeProgressItem = {
  id: 'local-201', sourceId: '201', seasonId: 'season-2', seasonNumber: 2, episodeNumber: 1,
  absoluteNumber: 4, name: '', watched: true, watchedAt,
}

const progress: SeriesProgress = {
  roundId: 'round-season-2', mediaId: 'series-1', seasonNumber: 2, status: 'watching', version: 3,
  watchedEpisodes: 1, totalEpisodes: 3, lastWatched: watchedEpisode, nextEpisode: null,
  episodes: [watchedEpisode],
}

const tvDetails = {
  id: 1399, name: '测试剧集', originalName: 'Test Series', firstAirDate: '2026-01-01',
  posterPath: '', backdropPath: '', overview: '', numberOfSeasons: 2, numberOfEpisodes: 6,
  episodeRuntime: [45], genres: ['剧情'],
  seasons: [
    { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2026-01-01', seasonNumber: 1, episodeCount: 3 },
    { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 3 },
  ],
}

const seasonDetails = {
  id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2,
  episodes: [
    { id: 201, name: '第四集', overview: '', airDate: '2026-07-01', seasonNumber: 2, episodeNumber: 1, runtime: 45, stillPath: '' },
    { id: 202, name: '第五集', overview: '', airDate: '2026-07-08', seasonNumber: 2, episodeNumber: 2, runtime: 45, stillPath: '' },
    { id: 203, name: '第六集', overview: '', airDate: '2026-07-15', seasonNumber: 2, episodeNumber: 3, runtime: 45, stillPath: '' },
  ],
}

beforeEach(() => {
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  server.use(
    http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json(tvDetails)),
    http.get('*/api/v1/tmdb/tv/1399/season/2', () => HttpResponse.json(seasonDetails)),
    http.get('*/api/v1/records/series-1/progress', ({ request }) => {
      expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
      return HttpResponse.json(progress)
    }),
  )
})

it('renders separate toggle and second-precision time controls for the selected season', async () => {
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={1399} seasonNumber={2} now={() => now} />)

  const watchedRow = (await screen.findByText('第四集')).closest('li')
  expect(watchedRow).not.toBeNull()
  expect(within(watchedRow!).getByRole('button', { name: '将 S02E01 标为未看' })).toBeVisible()
  expect(within(watchedRow!).getByRole('button', { name: /修改 S02E01 观看时间/ })).toHaveTextContent(formatLocalSeconds(watchedAt))

  const unwatchedRow = screen.getByText('第五集').closest('li')
  expect(within(unwatchedRow!).getByRole('button', { name: '标记 S02E02 已看' })).toBeVisible()
  expect(within(unwatchedRow!).getByRole('button', { name: '设置 S02E02 观看时间' })).toBeVisible()
})

it('records current time from an unwatched circle and undoes only the selected row', async () => {
  const bodies: Array<Record<string, unknown>> = []
  let current = progress
  server.use(http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
    expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
    const body = await request.json() as Record<string, unknown>
    bodies.push(body)
    const reference = (body.episodeRefs as Array<{ sourceId: string }>)[0]
    expect(reference).toBeDefined()
    if (!reference) return HttpResponse.json({ code: 'invalid_request' }, { status: 400 })
    if (body.action === 'single') {
      const episode = { ...watchedEpisode, id: 'local-202', sourceId: reference.sourceId, episodeNumber: 2, absoluteNumber: 5, watchedAt: now.toISOString() }
      current = { ...current, version: 4, watchedEpisodes: 2, episodes: [...current.episodes, episode] }
    } else {
      current = { ...current, version: 5, watchedEpisodes: 1, episodes: current.episodes.filter((item) => item.sourceId !== reference.sourceId) }
    }
    return HttpResponse.json(current)
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={1399} seasonNumber={2} now={() => now} />)

  await user.click(await screen.findByRole('button', { name: '标记 S02E02 已看' }))
  await waitFor(() => expect(bodies[0]).toMatchObject({
    action: 'single', expectedVersion: 3, watchedAt: now.toISOString(),
    episodeRefs: [{ sourceId: '202', seasonNumber: 2, episodeNumber: 2, absoluteNumber: 5 }],
    totalEpisodes: 3,
  }))

  await user.click(await screen.findByRole('button', { name: '将 S02E02 标为未看' }))
  await waitFor(() => expect(bodies[1]).toMatchObject({ action: 'undo', expectedVersion: 4 }))
})

it('sets an unwatched episode time and then edits the watched time in place', async () => {
  const bodies: Array<Record<string, unknown>> = []
  let version = 3
  server.use(http.post('*/api/v1/records/series-1/progress', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    bodies.push(body)
    version += 1
    const episode = {
      ...watchedEpisode,
      id: 'local-202', sourceId: '202', episodeNumber: 2, absoluteNumber: 5,
      watchedAt: body.watchedAt as string,
    }
    return HttpResponse.json({
      ...progress, version, watchedEpisodes: 2, lastWatched: episode,
      episodes: [watchedEpisode, episode],
    })
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<EpisodeProgress mediaId="series-1" tmdbId={1399} seasonNumber={2} now={() => now} />)

  await user.click(await screen.findByRole('button', { name: '设置 S02E02 观看时间' }))
  fireEvent.change(screen.getByLabelText('S02E02 观看时间'), { target: { value: '2026-07-12T11:10:12' } })
  await user.click(screen.getByRole('button', { name: '确定 S02E02 观看时间' }))
  await waitFor(() => expect(bodies[0]).toMatchObject({
    action: 'set_time', expectedVersion: 3, watchedAt: new Date(2026, 6, 12, 11, 10, 12).toISOString(),
  }))
  expect(await screen.findByRole('button', { name: /修改 S02E02 观看时间/ })).toHaveTextContent('2026-07-12 11:10:12')

  await user.click(screen.getByRole('button', { name: /修改 S02E02 观看时间/ }))
  fireEvent.change(screen.getByLabelText('S02E02 观看时间'), { target: { value: '2026-07-13T09:08:07' } })
  await user.click(screen.getByRole('button', { name: '确定 S02E02 观看时间' }))
  await waitFor(() => expect(bodies[1]).toMatchObject({ action: 'set_time', expectedVersion: 4 }))
  expect(await screen.findByRole('button', { name: /修改 S02E02 观看时间/ })).toHaveTextContent('2026-07-13 09:08:07')
})
