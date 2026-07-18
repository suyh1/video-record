import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { LibraryPage } from './LibraryPage'

describe('LibraryPage', () => {
  it('filters the private library by status and keeps poster information explicit', async () => {
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const status = new URL(request.url).searchParams.get('status')
        const items = [
          {
            id: 'media-1', source: 'local', mediaType: 'movie', title: '花样年华',
            originalTitle: 'In the Mood for Love', year: '2000', posterPath: null, status: 'completed',
          },
          {
            id: 'media-2', source: 'local', mediaType: 'tv', title: '漫长的季节',
            originalTitle: 'The Long Season', year: '2023', posterPath: null, status: 'wishlist',
          },
        ]
        return HttpResponse.json({ items: status === 'completed' ? items.slice(0, 1) : items, nextCursor: null })
      }),
    )
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>,
    )

    expect(await screen.findByText('In the Mood for Love')).toBeVisible()
    expect(screen.getByText('The Long Season')).toBeVisible()
    expect(screen.getByText('电影')).toBeVisible()
    expect(screen.getByText('剧集')).toBeVisible()
    expect(screen.getByLabelText('花样年华 暂无海报')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '看过' }))
    expect(await screen.findByRole('button', { name: '看过', pressed: true })).toBeVisible()
    expect(screen.getByText('In the Mood for Love')).toBeVisible()
    expect(screen.queryByText('The Long Season')).not.toBeInTheDocument()
  })

  it('opens search from the empty state primary action', async () => {
    server.use(http.get('*/api/v1/library', () => HttpResponse.json({ items: [], nextCursor: null })))
    const onSearch = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter>
        <LibraryPage onSearch={onSearch} />
      </MemoryRouter>,
    )

    await user.click(await screen.findByRole('button', { name: '搜索影视' }))

    expect(onSearch).toHaveBeenCalledOnce()
  })

  it('loads additional library pages with the returned cursor and keeps the cumulative count', async () => {
    const pageOne = Array.from({ length: 2 }, (_, index) => ({
      id: `media-${index + 1}`,
      source: 'local' as const,
      mediaType: 'movie' as const,
      title: `第一页${index + 1}`,
      originalTitle: `Page One ${index + 1}`,
      year: '2026',
      posterPath: null,
      status: 'wishlist' as const,
    }))
    const pageTwo = [{
      id: 'media-3',
      source: 'local' as const,
      mediaType: 'movie' as const,
      title: '第二页1',
      originalTitle: 'Page Two 1',
      year: '2026',
      posterPath: null,
      status: 'wishlist' as const,
    }]
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const cursor = new URL(request.url).searchParams.get('cursor')
        if (!cursor) {
          return HttpResponse.json({ items: pageOne, nextCursor: 'cursor-page-2' })
        }
        expect(cursor).toBe('cursor-page-2')
        return HttpResponse.json({ items: pageTwo, nextCursor: null })
      }),
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
    )
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>,
    )

    expect(await screen.findByText('Page One 1')).toBeVisible()
    expect(screen.getByText('2 部影视')).toBeVisible()
    expect(screen.queryByText('Page Two 1')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '加载更多' }))
    expect(await screen.findByText('Page Two 1')).toBeVisible()
    expect(screen.getByText('3 部影视')).toBeVisible()
    expect(screen.queryByRole('button', { name: '加载更多' })).not.toBeInTheDocument()
  })

  it('keeps status and collection filters mutually exclusive and counts displayed items', async () => {
    const items = [
      {
        id: 'media-1', source: 'local', mediaType: 'movie', title: '花样年华',
        originalTitle: 'In the Mood for Love', year: '2000', posterPath: null, status: 'completed',
      },
      {
        id: 'media-2', source: 'local', mediaType: 'tv', title: '漫长的季节',
        originalTitle: 'The Long Season', year: '2023', posterPath: null, status: 'wishlist',
      },
    ]
    server.use(
      http.get('*/api/v1/library', ({ request }) => {
        const status = new URL(request.url).searchParams.get('status')
        return HttpResponse.json({
          items: status === 'completed' ? items.slice(0, 1) : items,
          nextCursor: null,
        })
      }),
      http.get('*/api/v1/collections', () => HttpResponse.json([
        { id: 'collection-1', name: '周末电影', items: ['media-2'] },
      ])),
    )
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>,
    )

    await user.click(await screen.findByRole('button', { name: '周末电影，1 部影视' }))
    expect(screen.getByRole('button', { name: '周末电影，1 部影视', pressed: true })).toBeVisible()
    expect(screen.getByRole('button', { name: '全部', pressed: true })).toBeVisible()
    expect(screen.getByText('1 部影视')).toBeVisible()
    expect(screen.queryByText('In the Mood for Love')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '看过' }))
    expect(await screen.findByRole('button', { name: '看过', pressed: true })).toBeVisible()
    expect(screen.getByRole('button', { name: '周末电影，1 部影视', pressed: false })).toBeVisible()
    expect(screen.getByText('1 部影视')).toBeVisible()
    expect(screen.getByText('In the Mood for Love')).toBeVisible()
    expect(screen.queryByText('The Long Season')).not.toBeInTheDocument()
  })
})
