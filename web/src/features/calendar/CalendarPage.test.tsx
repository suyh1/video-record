import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { expect, it } from 'vitest'

import type { CalendarResponse } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { CalendarPage } from './CalendarPage'

const response: CalendarResponse = {
  year: 2026,
  month: 7,
  timezone: 'Asia/Shanghai',
  events: [
    {
      id: 'event-1', mediaId: 'movie-1', mediaType: 'movie', title: '一一',
      episodeId: null, seasonNumber: null, episodeNumber: null, absoluteNumber: null,
      watchedAt: '2026-07-12T15:00:00Z', localDate: '2026-07-12', viewingMethod: '家庭电视',
      participants: ['owner'], status: 'completed',
    },
    {
      id: 'event-2', mediaId: 'series-1', mediaType: 'tv', title: '漫长的季节',
      episodeId: 'episode-1', seasonNumber: 1, episodeNumber: 3, absoluteNumber: 3,
      watchedAt: '2026-07-13T12:00:00Z', localDate: '2026-07-13', viewingMethod: null,
      participants: ['owner', '家人'], status: 'watching',
    },
    {
      id: 'event-3', mediaId: 'movie-2', mediaType: 'movie', title: '花样年华',
      episodeId: null, seasonNumber: null, episodeNumber: null, absoluteNumber: null,
      watchedAt: '2026-07-13T14:00:00Z', localDate: '2026-07-13', viewingMethod: '影院',
      participants: ['owner'], status: 'completed',
    },
  ],
}

it('groups repeated watches in the month grid and orders the mobile agenda newest first', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const month = await screen.findByRole('table', { name: '2026年7月观影日历' })
  const day = within(month).getByRole('cell', { name: '7月13日，2 条记录' })
  expect(day).toHaveTextContent('漫长的季节')
  expect(day).toHaveTextContent('花样年华')

  const agenda = screen.getByRole('list', { name: '按日议程' })
  const days = within(agenda).getAllByRole('listitem', { name: /月/ })
  const [firstDay, secondDay] = days
  if (!firstDay || !secondDay) throw new Error('expected two agenda days')
  expect(firstDay).toHaveAccessibleName('7月13日')
  expect(secondDay).toHaveAccessibleName('7月12日')
  expect(within(firstDay).getByText('S01E03 · 全剧第 3 集')).toBeVisible()
  expect(within(firstDay).getByText('与 owner、家人共同观看')).toBeVisible()
})

it('requests completed and in-progress filters without losing the selected month', async () => {
  const urls: string[] = []
  server.use(http.get('*/api/v1/calendar', ({ request }) => {
    urls.push(request.url)
    return HttpResponse.json(response)
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)
  await screen.findByRole('table', { name: '2026年7月观影日历' })

  await user.click(screen.getByRole('button', { name: '只看进行中' }))
  expect(await screen.findByRole('button', { name: '只看进行中', pressed: true })).toBeVisible()
  expect(urls.at(-1)).toContain('month=2026-07')
  expect(urls.at(-1)).toContain('filter=in_progress')
  expect(urls.at(-1)).toContain('timezone=Asia%2FShanghai')
})

it('retries a failed month without changing the current selection', async () => {
  let attempts = 0
  server.use(http.get('*/api/v1/calendar', () => {
    attempts += 1
    return attempts === 1
      ? HttpResponse.json({ code: 'calendar_unavailable' }, { status: 503 })
      : HttpResponse.json(response)
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  expect(await screen.findByRole('alert')).toHaveTextContent('无法读取日历')
  await user.click(screen.getByRole('button', { name: '重试日历' }))

  expect(await screen.findByRole('table', { name: '2026年7月观影日历' })).toBeVisible()
  expect(screen.getByRole('heading', { name: '2026年7月' })).toBeVisible()
  expect(attempts).toBe(2)
})
