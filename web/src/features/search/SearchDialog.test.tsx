import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'

import { server } from '../../test/server'
import { renderWithQueryClient } from '../../test/render'
import { SearchDialog } from './SearchDialog'

describe('SearchDialog', () => {
  it('debounces for 300ms, shows local results first, then merges labeled TMDB results', async () => {
    let localCalls = 0
    let remoteCalls = 0
    server.use(
      http.get('*/api/v1/media/search', ({ request }) => {
        localCalls += 1
        expect(new URL(request.url).searchParams.get('q')).toBe('沙丘')
        return HttpResponse.json({
          items: [
            {
              id: 'local-dune',
              source: 'local',
              mediaType: 'movie',
              title: '沙丘',
              originalTitle: 'Dune',
              year: '2021',
              posterPath: null,
              status: 'completed',
            },
          ],
        })
      }),
      http.get('*/api/v1/tmdb/search', async () => {
        remoteCalls += 1
        await delay(200)
        return HttpResponse.json({
          results: [
            {
              id: 693134,
              mediaType: 'movie',
              title: '沙丘：第二部',
              originalTitle: 'Dune: Part Two',
              year: '2024',
              posterPath: '/poster.jpg',
            },
          ],
        })
      }),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '沙丘')
    await new Promise((resolve) => setTimeout(resolve, 250))
    expect(localCalls).toBe(0)
    expect(remoteCalls).toBe(0)

    expect(await screen.findByText('Dune')).toBeVisible()
    expect(screen.getByText('2021')).toBeVisible()
    expect(screen.getByText('电影')).toBeVisible()
    expect(screen.getByText('看过')).toBeVisible()
    expect(screen.queryByText('Dune: Part Two')).not.toBeInTheDocument()

    expect(await screen.findByText('Dune: Part Two')).toBeVisible()
    expect(screen.getByText('2024')).toBeVisible()
    await waitFor(() => expect(localCalls).toBe(1))
    expect(remoteCalls).toBe(1)
  })

  it('offers a custom item only after local and TMDB searches return no results', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
    let createBody: unknown
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
      http.post('*/api/v1/media/custom', async ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        createBody = await request.json()
        return HttpResponse.json({
          id: 'custom-1', mediaType: 'tv', title: '私藏短剧', overview: '',
          externalTitle: '', externalOverview: '', originalTitle: '', releaseDate: '2025',
          posterPath: '', backdropPath: '', runtimeMinutes: 0, genres: [],
        }, { status: 201 })
      }),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '私藏短剧')
    await screen.findByText('没有找到匹配的电影或剧集')
    await user.click(screen.getByRole('button', { name: '创建自定义条目' }))
    await user.click(screen.getByRole('radio', { name: '剧集' }))
    await user.type(screen.getByRole('textbox', { name: '年份（可选）' }), '2025')
    await user.click(screen.getByRole('button', { name: '保存自定义条目' }))

    await waitFor(() => expect(createBody).toEqual({ title: '私藏短剧', mediaType: 'tv', year: '2025', overview: '' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({
      id: 'custom-1', source: 'local', mediaType: 'tv', title: '私藏短剧', year: '2025',
    }))
  })
})
