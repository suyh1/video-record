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

it('defaults the mobile view control to agenda and keeps both responsive views mounted', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const viewControl = await screen.findByRole('group', { name: '日历视图' })
  const agendaButton = within(viewControl).getByRole('button', { name: '日程', pressed: true })
  const monthButton = within(viewControl).getByRole('button', { name: '月历', pressed: false })
  expect(agendaButton).toHaveAttribute('aria-controls', 'calendar-agenda-view')
  expect(monthButton).toHaveAttribute('aria-controls', 'calendar-month-view')
  expect(await screen.findByRole('list', { name: '按日议程' })).toBeInTheDocument()
  expect(await screen.findByRole('table', { name: '2026年7月观影日历' })).toBeInTheDocument()

  await user.click(monthButton)

  expect(monthButton).toHaveAttribute('aria-pressed', 'true')
  expect(agendaButton).toHaveAttribute('aria-pressed', 'false')
  expect(screen.getByRole('table', { name: '2026年7月观影日历' })).toBeInTheDocument()
})

it('links a selected calendar date to its agenda and restores the full month', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const viewControl = await screen.findByRole('group', { name: '日历视图' })
  await user.click(within(viewControl).getByRole('button', { name: '月历' }))
  const month = await screen.findByRole('table', { name: '2026年7月观影日历' })
  const selectedDay = within(month).getByRole('button', { name: '7月13日，2 条记录' })
  expect(selectedDay).toHaveAttribute('aria-current', 'date')
  expect(selectedDay).toHaveAttribute('aria-pressed', 'false')
  expect(selectedDay).toHaveAttribute('data-has-events', 'true')

  await user.click(selectedDay)

  expect(selectedDay).toHaveAttribute('aria-pressed', 'true')
  expect(within(viewControl).getByRole('button', { name: '日程', pressed: true })).toBeVisible()
  const agenda = screen.getByRole('list', { name: '按日议程' })
  expect(within(agenda).getAllByRole('link')).toHaveLength(2)
  expect(within(agenda).getByText('漫长的季节')).toBeVisible()
  expect(within(agenda).getByText('花样年华')).toBeVisible()
  expect(within(agenda).queryByText('一一')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: '查看全月' }))

  expect(within(screen.getByRole('list', { name: '按日议程' })).getAllByRole('link')).toHaveLength(3)
  expect(screen.queryByRole('button', { name: '查看全月' })).not.toBeInTheDocument()
})

it('explains an empty selected day instead of rendering an empty agenda', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const month = await screen.findByRole('table', { name: '2026年7月观影日历' })
  await user.click(within(month).getByRole('button', { name: '7月14日，0 条记录' }))

  expect(screen.getByRole('status')).toHaveTextContent('7月14日暂无观看记录')
  expect(screen.queryByRole('list', { name: '按日议程' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '查看全月' })).toBeVisible()
})

it('keeps an empty selected day mounted inside the mobile agenda view', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const viewControl = await screen.findByRole('group', { name: '日历视图' })
  const month = screen.getByRole('table', { name: '2026年7月观影日历' })
  await user.click(within(month).getByRole('button', { name: '7月14日，0 条记录' }))

  const agendaViews = document.querySelectorAll('#calendar-agenda-view')
  expect(agendaViews).toHaveLength(1)
  const [agendaView] = agendaViews
  if (!(agendaView instanceof HTMLElement)) throw new Error('expected an agenda view')
  expect(agendaView).toHaveClass('is-active')
  expect(within(agendaView).getByRole('status')).toHaveTextContent('7月14日暂无观看记录')

  await user.click(within(viewControl).getByRole('button', { name: '月历' }))

  expect(agendaView).not.toHaveClass('is-active')
  expect(within(agendaView).getByRole('status')).toBeInTheDocument()

  await user.click(within(viewControl).getByRole('button', { name: '日程' }))

  expect(agendaView).toHaveClass('is-active')
})

it('moves focus to the agenda view after selecting a date with the keyboard', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const viewControl = await screen.findByRole('group', { name: '日历视图' })
  await user.click(within(viewControl).getByRole('button', { name: '月历' }))
  const month = screen.getByRole('table', { name: '2026年7月观影日历' })
  const selectedDay = within(month).getByRole('button', { name: '7月13日，2 条记录' })
  selectedDay.focus()

  await user.keyboard('{Enter}')

  expect(within(viewControl).getByRole('button', { name: '日程', pressed: true })).toBeVisible()
  expect(document.querySelector('#calendar-agenda-view')).toHaveFocus()
})

it('keeps focus in the agenda view after restoring the full month', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json(response)))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const month = await screen.findByRole('table', { name: '2026年7月观影日历' })
  await user.click(within(month).getByRole('button', { name: '7月13日，2 条记录' }))
  const agendaView = document.querySelector('#calendar-agenda-view')
  expect(agendaView).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: '查看全月' }))

  expect(agendaView).toHaveFocus()
  expect(document.body).not.toHaveFocus()
})

it('clears a selected date when moving to another month', async () => {
  server.use(http.get('*/api/v1/calendar', ({ request }) => {
    const requestedMonth = new URL(request.url).searchParams.get('month')
    return HttpResponse.json(requestedMonth === '2026-08'
      ? { ...response, month: 8, events: [] }
      : response)
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const month = await screen.findByRole('table', { name: '2026年7月观影日历' })
  await user.click(within(month).getByRole('button', { name: '7月13日，2 条记录' }))
  expect(screen.getByRole('button', { name: '查看全月' })).toBeVisible()

  await user.click(screen.getByRole('button', { name: '下个月' }))

  expect(await screen.findByRole('heading', { name: '2026年8月' })).toBeVisible()
  expect(await screen.findByRole('region', { name: '日历暂无记录' })).toBeVisible()
  expect(screen.queryByRole('button', { name: '查看全月' })).not.toBeInTheDocument()
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

it('offers exactly one named action when the selected month is empty', async () => {
  server.use(http.get('*/api/v1/calendar', () => HttpResponse.json({ ...response, events: [] })))
  renderWithQueryClient(<MemoryRouter><CalendarPage now={new Date('2026-07-13T12:00:00Z')} timezone="Asia/Shanghai" /></MemoryRouter>)

  const empty = await screen.findByRole('region', { name: '日历暂无记录' })
  expect(empty.querySelector('[data-brand-mark="film-archive"]')).toBeInTheDocument()
  expect(within(empty).getAllByRole('link')).toHaveLength(1)
  expect(within(empty).getByRole('link', { name: '去影库记录' })).toHaveAttribute('href', '/library')
  expect(within(empty).queryByRole('button')).not.toBeInTheDocument()
})
