import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { server } from '../../test/server'
import { renderWithQueryClient } from '../../test/render'
import { SearchDialog } from './SearchDialog'

const recentSearchesKey = 'video-record.recent-searches'
const signature = 'b'.repeat(64)

beforeEach(() => {
  sessionStorage.removeItem(recentSearchesKey)
  localStorage.clear()
})

afterEach(() => vi.restoreAllMocks())

describe('SearchDialog', () => {
  it('debounces for 300ms and renders deduplicated local and TMDB sections', async () => {
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
              tmdbId: 693134,
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
              title: '沙丘',
              originalTitle: 'Dune',
              year: '2021',
              posterPath: `/api/v1/public/tmdb/images/w342/duplicate.jpg?expires=4102444800&signature=${signature}`,
            },
            {
              id: 693135,
              mediaType: 'movie',
              title: '沙丘：第二部',
              originalTitle: 'Dune: Part Two',
              year: '2024',
              posterPath: `/api/v1/public/tmdb/images/w342/dune-two.jpg?expires=4102444800&signature=${signature}`,
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

    const localSection = await screen.findByRole('region', { name: '本地影库' })
    expect(await within(localSection).findByText('Dune')).toBeVisible()
    expect(within(localSection).getByText('2021')).toBeVisible()
    expect(within(localSection).getByText('看过')).toBeVisible()
    const remoteSection = screen.getByRole('region', { name: 'TMDB' })
    expect(within(remoteSection).queryByText('Dune: Part Two')).not.toBeInTheDocument()

    expect(await within(remoteSection).findByText('Dune: Part Two')).toBeVisible()
    expect(within(remoteSection).getByText('2024')).toBeVisible()
    expect(screen.getAllByText('Dune')).toHaveLength(1)
    await waitFor(() => expect(localCalls).toBe(1))
    expect(remoteCalls).toBe(1)
  })

  it('hides settled results immediately while a changed query is still debouncing', async () => {
    server.use(
      http.get('*/api/v1/media/search', ({ request }) => {
        const query = new URL(request.url).searchParams.get('q') ?? ''
        return HttpResponse.json({ items: [{
          id: `local-${query}`, source: 'local', mediaType: 'movie', title: `${query}结果`, originalTitle: '',
          year: '', posterPath: null, status: 'none',
        }] })
      }),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    const input = screen.getByRole('searchbox', { name: '搜索影视' })
    await user.type(input, '旧词')
    expect(await screen.findByRole('button', { name: /旧词结果/ })).toBeVisible()

    await user.clear(input)
    await user.type(input, '新词')

    expect(screen.queryByRole('button', { name: /旧词结果/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: '本地影库' })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'TMDB' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('正在搜索')).toBeVisible()
    expect(onSelect).not.toHaveBeenCalled()
    expect(await screen.findByRole('button', { name: /新词结果/ })).toBeVisible()
  })

  it('moves focus across result sections with arrow keys, selects with Enter, and closes with Escape', async () => {
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [{
        id: 'local-1', source: 'local', mediaType: 'movie', title: '本地电影', originalTitle: '',
        year: '2020', posterPath: null, status: 'watching',
      }] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [{
        id: 22, mediaType: 'tv', title: '远端剧集', originalTitle: '', year: '2024', posterPath: null,
      }] })),
    )
    const onClose = vi.fn()
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={onClose} onSelect={onSelect} />)

    const input = screen.getByRole('searchbox', { name: '搜索影视' })
    await user.type(input, '电影')
    const localButton = await screen.findByRole('button', { name: /本地电影/ })
    const remoteButton = await screen.findByRole('button', { name: /远端剧集/ })

    input.focus()
    await user.keyboard('{ArrowDown}')
    expect(document.activeElement).toBe(localButton)
    await user.keyboard('{ArrowDown}')
    expect(document.activeElement).toBe(remoteButton)
    await user.keyboard('{ArrowUp}')
    expect(document.activeElement).toBe(localButton)
    await user.keyboard('{Enter}')
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'local-1' }))

    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('keeps TMDB results operable when local search fails', async () => {
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ code: 'unavailable' }, { status: 503 })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [{
        id: 23, mediaType: 'movie', title: '仍可选择', originalTitle: '', year: '2023', posterPath: null,
      }] })),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '测试')
    expect(await screen.findByRole('alert', { name: '本地影库搜索失败' })).toBeVisible()
    await user.click(await screen.findByRole('button', { name: /仍可选择/ }))

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ title: '仍可选择' }))
    expect(screen.queryByRole('button', { name: '创建自定义条目' })).not.toBeInTheDocument()
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
    expect(screen.getByRole('region', { name: '本地影库' })).toBeVisible()
    expect(screen.getByRole('region', { name: 'TMDB' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: '创建自定义条目' }))
    await user.click(screen.getByRole('radio', { name: '剧集' }))
    await user.type(screen.getByRole('textbox', { name: '年份（可选）' }), '2025')
    await user.click(screen.getByRole('button', { name: '保存自定义条目' }))

    await waitFor(() => expect(createBody).toEqual({ title: '私藏短剧', mediaType: 'tv', year: '2025', overview: '' }))
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({
      id: 'custom-1', source: 'local', mediaType: 'tv', title: '私藏短剧', year: '2025',
    }))
  })

  it('ignores a custom creation response that arrives after the dialog closes', async () => {
    let markStarted: (() => void) | undefined
    let releaseResponse: (() => void) | undefined
    const started = new Promise<void>((resolve) => { markStarted = resolve })
    const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve })
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
      http.post('*/api/v1/media/custom', async () => {
        markStarted?.()
        await responseGate
        return HttpResponse.json({
          id: 'late-custom', mediaType: 'movie', title: '迟到条目', overview: '', externalTitle: '',
          externalOverview: '', originalTitle: '', releaseDate: '2026', posterPath: '', backdropPath: '',
          runtimeMinutes: 0, genres: [],
        }, { status: 201 })
      }),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    const { rerender } = renderWithQueryClient(
      <SearchDialog open onClose={() => undefined} onSelect={onSelect} />,
    )

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '迟到条目')
    await user.click(await screen.findByRole('button', { name: '创建自定义条目' }))
    await user.click(screen.getByRole('button', { name: '保存自定义条目' }))
    await started
    rerender(<SearchDialog open={false} onClose={() => undefined} onSelect={onSelect} />)
    releaseResponse?.()
    await new Promise((resolve) => setTimeout(resolve, 50))

    expect(onSelect).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(recentSearchesKey)).toBeNull()
  })

  it('ignores a pending custom creation response after the search query changes', async () => {
    let markStarted: (() => void) | undefined
    let releaseResponse: (() => void) | undefined
    const started = new Promise<void>((resolve) => { markStarted = resolve })
    const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve })
    server.use(
      http.get('*/api/v1/media/search', ({ request }) => {
        const query = new URL(request.url).searchParams.get('q') ?? ''
        return HttpResponse.json({ items: query === '新搜索' ? [{
          id: 'new-result', source: 'local', mediaType: 'movie', title: '新搜索结果', originalTitle: '',
          year: '2026', posterPath: null, status: 'none',
        }] : [] })
      }),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
      http.post('*/api/v1/media/custom', async () => {
        markStarted?.()
        await responseGate
        return HttpResponse.json({
          id: 'stale-custom', mediaType: 'movie', title: '旧自定义', overview: '', externalTitle: '',
          externalOverview: '', originalTitle: '', releaseDate: '2026', posterPath: '', backdropPath: '',
          runtimeMinutes: 0, genres: [],
        }, { status: 201 })
      }),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    const input = screen.getByRole('searchbox', { name: '搜索影视' })
    await user.type(input, '旧自定义')
    await user.click(await screen.findByRole('button', { name: '创建自定义条目' }))
    await user.click(screen.getByRole('button', { name: '保存自定义条目' }))
    await started
    await user.clear(input)
    await user.type(input, '新搜索')
    const newResult = await screen.findByRole('button', { name: /新搜索结果/ })
    releaseResponse?.()
    await new Promise((resolve) => setTimeout(resolve, 50))

    expect(onSelect).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(recentSearchesKey)).toBeNull()
    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
    expect(input).toHaveValue('新搜索')
    expect(newResult).toBeVisible()
  })

  it('does not let an old custom response affect a reopened dialog', async () => {
    let markStarted: (() => void) | undefined
    let releaseResponse: (() => void) | undefined
    const started = new Promise<void>((resolve) => { markStarted = resolve })
    const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve })
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
      http.post('*/api/v1/media/custom', async () => {
        markStarted?.()
        await responseGate
        return HttpResponse.json({
          id: 'old-custom', mediaType: 'movie', title: '旧响应', overview: '', externalTitle: '',
          externalOverview: '', originalTitle: '', releaseDate: '2026', posterPath: '', backdropPath: '',
          runtimeMinutes: 0, genres: [],
        }, { status: 201 })
      }),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    const { rerender } = renderWithQueryClient(
      <SearchDialog open onClose={() => undefined} onSelect={onSelect} />,
    )

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '旧响应')
    await user.click(await screen.findByRole('button', { name: '创建自定义条目' }))
    await user.click(screen.getByRole('button', { name: '保存自定义条目' }))
    await started
    rerender(<SearchDialog open={false} onClose={() => undefined} onSelect={onSelect} />)
    rerender(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)
    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toHaveValue('旧响应')
    releaseResponse?.()
    await new Promise((resolve) => setTimeout(resolve, 50))

    expect(onSelect).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(recentSearchesKey)).toBeNull()
    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
  })

  it('does not expose a late custom error in a reopened dialog', async () => {
    let markStarted: (() => void) | undefined
    let releaseResponse: (() => void) | undefined
    const started = new Promise<void>((resolve) => { markStarted = resolve })
    const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve })
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
      http.post('*/api/v1/media/custom', async () => {
        markStarted?.()
        await responseGate
        return HttpResponse.json({ code: 'late_failure' }, { status: 500 })
      }),
    )
    const user = userEvent.setup()
    const { rerender } = renderWithQueryClient(
      <SearchDialog open onClose={() => undefined} onSelect={() => undefined} />,
    )

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '失败旧请求')
    await user.click(await screen.findByRole('button', { name: '创建自定义条目' }))
    await user.click(screen.getByRole('button', { name: '保存自定义条目' }))
    await started
    rerender(<SearchDialog open={false} onClose={() => undefined} onSelect={() => undefined} />)
    rerender(<SearchDialog open onClose={() => undefined} onSelect={() => undefined} />)
    releaseResponse?.()
    await new Promise((resolve) => setTimeout(resolve, 50))
    await user.click(screen.getByRole('button', { name: '创建自定义条目' }))

    expect(screen.queryByText('创建失败，标题、类型和年份仍保留。')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存自定义条目' })).toBeEnabled()
  })

  it('stores only successful queries in bounded deduplicated session history without backend or local storage writes', async () => {
    sessionStorage.setItem(recentSearchesKey, JSON.stringify(['旧一', '旧二', '旧三', '旧四', '旧五']))
    const originalSetItem = Storage.prototype.setItem
    const localStorageWrites: string[] = []
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (this: Storage, key, value) {
      if (this === window.localStorage) localStorageWrites.push(key)
      return originalSetItem.call(this, key, value)
    })
    let backendWrites = 0
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [{
        id: 'local-old', source: 'local', mediaType: 'movie', title: '旧二', originalTitle: '',
        year: '2020', posterPath: null, status: 'completed',
      }] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
      http.post('*/api/v1/*', () => {
        backendWrites += 1
        return HttpResponse.json({}, { status: 500 })
      }),
    )
    const user = userEvent.setup()
    const onSelect = vi.fn()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '旧二')
    await user.click(await screen.findByRole('button', { name: /旧二/ }))

    expect(JSON.parse(sessionStorage.getItem(recentSearchesKey) ?? '[]')).toEqual(['旧二', '旧一', '旧三', '旧四', '旧五'])
    expect(localStorageWrites).toEqual([])
    expect(backendWrites).toBe(0)
  })

  it('records the settled query even when input changes while selection is pending', async () => {
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [{
        id: 'snapshot-1', source: 'local', mediaType: 'movie', title: '快照条目', originalTitle: '',
        year: '', posterPath: null, status: 'none',
      }] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    let resolveSelection: (() => void) | undefined
    const selection = new Promise<void>((resolve) => { resolveSelection = resolve })
    const user = userEvent.setup()
    renderWithQueryClient(
      <SearchDialog open onClose={() => undefined} onSelect={() => selection} />,
    )

    const input = screen.getByRole('searchbox', { name: '搜索影视' })
    await user.type(input, '已提交查询')
    await user.click(await screen.findByRole('button', { name: /快照条目/ }))
    await user.clear(input)
    await user.type(input, '后来输入')
    await act(async () => resolveSelection?.())

    await waitFor(() => expect(JSON.parse(sessionStorage.getItem(recentSearchesKey) ?? '[]')).toEqual(['已提交查询']))
  })

  it('shows recent searches for an empty query, refills one to search, and clears history', async () => {
    sessionStorage.setItem(recentSearchesKey, JSON.stringify(['银翼杀手']))
    let searchedQuery = ''
    server.use(
      http.get('*/api/v1/media/search', ({ request }) => {
        searchedQuery = new URL(request.url).searchParams.get('q') ?? ''
        return HttpResponse.json({ items: [] })
      }),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={() => undefined} />)

    const recent = screen.getByRole('region', { name: '最近搜索' })
    await user.click(within(recent).getByRole('button', { name: '银翼杀手' }))
    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toHaveValue('银翼杀手')
    await waitFor(() => expect(searchedQuery).toBe('银翼杀手'))

    await user.clear(screen.getByRole('searchbox', { name: '搜索影视' }))
    expect(screen.queryByRole('region', { name: '本地影库' })).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'TMDB' })).not.toBeInTheDocument()
    await user.click(await screen.findByRole('button', { name: '清除最近搜索' }))
    expect(sessionStorage.getItem(recentSearchesKey)).toBeNull()
    expect(screen.queryByRole('region', { name: '最近搜索' })).not.toBeInTheDocument()
  })

  it('prevents duplicate selection and does not leak a pending selection across close and reopen', async () => {
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [{
        id: 'pending-1', source: 'local', mediaType: 'movie', title: '等待选择', originalTitle: '',
        year: '', posterPath: null, status: 'none',
      }] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    let rejectSelection: ((reason?: unknown) => void) | undefined
    const selection = new Promise<void>((_, reject) => { rejectSelection = reject })
    const onSelect = vi.fn(() => selection)
    const user = userEvent.setup()
    const { rerender } = renderWithQueryClient(
      <SearchDialog open onClose={() => undefined} onSelect={onSelect} />,
    )

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '等待')
    const result = await screen.findByRole('button', { name: /等待选择/ })
    await user.click(result)
    expect(result).not.toBeDisabled()
    expect(result).toHaveAttribute('aria-disabled', 'true')
    expect(document.activeElement).toBe(result)
    await user.click(result)
    expect(onSelect).toHaveBeenCalledOnce()

    rerender(<SearchDialog open={false} onClose={() => undefined} onSelect={onSelect} />)
    rerender(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)
    const reopenedResult = await screen.findByRole('button', { name: /等待选择/ })
    await waitFor(() => expect(reopenedResult).toHaveAttribute('aria-disabled', 'false'))
    rejectSelection?.(new Error('stale failure'))
    await Promise.resolve()
    expect(screen.queryByText('无法打开这个结果，搜索内容已保留。')).not.toBeInTheDocument()
  })

  it('preserves the query and reports a current selection failure', async () => {
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [{
        id: 'failed-1', source: 'local', mediaType: 'movie', title: '失败条目', originalTitle: '',
        year: '', posterPath: null, status: 'none',
      }] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    const user = userEvent.setup()
    renderWithQueryClient(
      <SearchDialog open onClose={() => undefined} onSelect={() => Promise.reject(new Error('failed'))} />,
    )

    const input = screen.getByRole('searchbox', { name: '搜索影视' })
    await user.type(input, '保留查询')
    const failedResult = await screen.findByRole('button', { name: /失败条目/ })
    await user.click(failedResult)

    expect(await screen.findByRole('alert')).toHaveTextContent('无法打开这个结果，搜索内容已保留。')
    expect(input).toHaveValue('保留查询')
    expect(failedResult).toHaveAttribute('aria-disabled', 'false')
    expect(document.activeElement).toBe(failedResult)
  })

  it('keeps selection usable when session storage quota rejects recent history', async () => {
    const originalSetItem = Storage.prototype.setItem
    const storageSet = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (this: Storage, key, value) {
      if (this === window.sessionStorage) throw new DOMException('quota exceeded', 'QuotaExceededError')
      return originalSetItem.call(this, key, value)
    })
    server.use(
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [{
        id: 'quota-1', source: 'local', mediaType: 'movie', title: '配额条目', originalTitle: '',
        year: '', posterPath: null, status: 'none',
      }] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    const onSelect = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={onSelect} />)

    await user.type(screen.getByRole('searchbox', { name: '搜索影视' }), '配额')
    await user.click(await screen.findByRole('button', { name: /配额条目/ }))

    expect(onSelect).toHaveBeenCalledOnce()
    expect(storageSet).toHaveBeenCalledWith(recentSearchesKey, JSON.stringify(['配额']))
    expect(screen.queryByText('无法打开这个结果，搜索内容已保留。')).not.toBeInTheDocument()
  })

  it('degrades safely when recent-search session storage contains invalid JSON', () => {
    sessionStorage.setItem(recentSearchesKey, '{invalid')

    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={() => undefined} />)

    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
    expect(screen.queryByRole('region', { name: '最近搜索' })).not.toBeInTheDocument()
  })

  it('normalizes duplicated and oversized recent-search session data', () => {
    sessionStorage.setItem(recentSearchesKey, JSON.stringify([
      '银翼杀手', ' 银翼杀手 ', '沙丘', '', '降临', '花样年华', '一一', '第六条',
    ]))

    renderWithQueryClient(<SearchDialog open onClose={() => undefined} onSelect={() => undefined} />)

    const recent = screen.getByRole('region', { name: '最近搜索' })
    expect(within(recent).getAllByRole('button').map((button) => button.textContent)).toEqual([
      '', '银翼杀手', '沙丘', '降临', '花样年华', '一一',
    ])
  })
})
