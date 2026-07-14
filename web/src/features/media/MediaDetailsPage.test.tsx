import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { MediaDetailsPage } from './MediaDetailsPage'

describe('MediaDetailsPage', () => {
  it('shows a live cinematic header, cast, personal record, and folded household tools', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
    let recordVersion = 3
    let tags = ['家庭', '怀旧']
    let tagsRequest: unknown
    let watchEvents = [
      {
        id: 'event-1', mediaId: 'media-1', watchedAt: '2026-07-12T20:30:00Z',
        viewingMethod: '家庭投影', source: 'manual', completion: 100,
      },
    ]
    let deletedEvent = ''
    server.use(
      http.get('*/api/v1/media/media-1', () =>
        HttpResponse.json({
          id: 'media-1',
          tmdbId: 329865,
          mediaType: 'movie',
          title: '花样年华',
          originalTitle: 'In the Mood for Love',
          releaseDate: '2000-09-29',
          overview: '两位邻居在克制与靠近之间建立起一段关系。',
          externalTitle: '花样年华',
          externalOverview: '两位邻居在克制与靠近之间建立起一段关系。',
          posterPath: null,
          backdropPath: '',
          runtimeMinutes: 0,
          genres: [],
        }),
      ),
      http.get('*/api/v1/tmdb/movie/329865', () => HttpResponse.json({
        id: 329865, title: '花样年华', originalTitle: 'In the Mood for Love', releaseDate: '2000-09-29',
        posterPath: '/mood.jpg', backdropPath: '/mood-backdrop.jpg',
        overview: '两位邻居在克制与靠近之间建立起一段关系。', runtime: 98, genres: ['剧情', '爱情'],
      })),
      http.get('*/api/v1/tmdb/movie/329865/credits', () => HttpResponse.json({
        cast: [
          { id: 1, name: '梁朝伟', character: '周慕云', profilePath: '/leung.jpg', order: 0 },
          { id: 2, name: '张曼玉', character: '苏丽珍', profilePath: '', order: 1 },
        ],
      })),
      http.get('*/api/v1/records/media-1', () =>
        HttpResponse.json({
          mediaId: 'media-1', status: 'completed', rating: 9.4, note: '雨夜与走廊。',
          watchedAt: '2026-07-12T12:00:00Z', viewingMethod: '家庭投影', version: recordVersion,
        }),
      ),
      http.get('*/api/v1/records/media-1/tags', () => HttpResponse.json({ tags })),
      http.put('*/api/v1/records/media-1/tags', async ({ request }) => {
        expect(request.headers.get('If-Match')).toBe('"3"')
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        tagsRequest = await request.json()
        tags = ['家庭', '经典']
        recordVersion = 4
        return new HttpResponse(null, { status: 204, headers: { ETag: '"4"' } })
      }),
      http.get('*/api/v1/records/media-1/events', () =>
        HttpResponse.json(watchEvents),
      ),
      http.delete('*/api/v1/records/media-1/events/event-1', ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        deletedEvent = 'event-1'
        watchEvents = []
        return new HttpResponse(null, { status: 204 })
      }),
      http.get('*/api/v1/household/participants', () =>
        HttpResponse.json([
          { id: 'member-1', username: 'family', role: 'member', active: true },
        ]),
      ),
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
      http.get('*/api/v1/household/records/media-1/sharing', () => HttpResponse.json({
        mediaId: 'media-1', shareRating: false, shareReview: false, sharedReview: null, version: recordVersion,
      })),
      http.get('*/api/v1/household/records/member-1/media-1', () => HttpResponse.json({
        ownerId: 'member-1', mediaId: 'media-1', rating: 8.6,
        privateNote: null, sharedReview: '适合一家人一起看',
      })),
    )
    renderWithQueryClient(
      <main>
        <MemoryRouter initialEntries={['/media/media-1']}>
          <Routes>
            <Route path="/media/:mediaId" element={<MediaDetailsPage />} />
          </Routes>
        </MemoryRouter>
      </main>,
    )

    expect(await screen.findByRole('heading', { name: '花样年华', level: 1 })).toBeVisible()
    expect(await screen.findByRole('img', { name: '花样年华 背景' })).toHaveAttribute('src', expect.stringContaining('/w1280/mood-backdrop.jpg'))
    expect(await screen.findByRole('img', { name: '花样年华 海报' })).toBeVisible()
    expect(await screen.findByText('98 分钟')).toBeVisible()
    expect(await screen.findByText('爱情')).toBeVisible()
    expect(screen.getByRole('region', { name: '主要演员' })).toBeVisible()
    expect(await screen.findByText('梁朝伟')).toBeVisible()
    expect(screen.getByText('周慕云')).toBeVisible()
    expect(screen.getByText('张曼玉')).toBeVisible()
    expect(screen.getAllByText('9.4 / 10')).toHaveLength(2)
    expect(screen.getByText('雨夜与走廊。')).toBeVisible()
    expect(screen.getByText('2026年7月12日')).toBeVisible()
    expect(screen.queryByRole('checkbox', { name: '向家庭公开评分' })).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: '私人标签' })).not.toBeInTheDocument()
    const user = userEvent.setup()
    await user.click(screen.getByText('家庭与整理'))
    expect(await screen.findByRole('checkbox', { name: 'family' })).toBeVisible()
    const householdShared = await screen.findByRole('region', { name: '家庭评价' })
    expect(within(householdShared).getByText('family')).toBeVisible()
    expect(within(householdShared).getByText('8.6 / 10')).toBeVisible()
    expect(within(householdShared).getByText('适合一家人一起看')).toBeVisible()
    const tagInput = await screen.findByRole('textbox', { name: '私人标签' })
    expect(tagInput).toHaveValue('家庭, 怀旧')
    await user.clear(tagInput)
    await user.type(tagInput, '家庭，经典')
    await user.click(screen.getByRole('button', { name: '保存标签' }))
    await waitFor(() => expect(tagsRequest).toEqual({ tags: ['家庭', '经典'] }))
    expect(screen.getByRole('status')).toHaveTextContent('标签已保存')

    await user.click(screen.getByRole('button', { name: '删除 2026年7月12日的观看事件' }))
    const dialog = screen.getByRole('dialog', { name: '删除观看事件' })
    expect(dialog).toHaveTextContent('评分、笔记和标签不会被删除')
    const confirmDelete = screen.getByRole('button', { name: '确认删除观看事件' })
    expect(confirmDelete).toHaveFocus()
    await user.click(confirmDelete)
    await waitFor(() => expect(deletedEvent).toBe('event-1'))
    expect(await screen.findByText('还没有观看事件')).toBeVisible()
    expect(screen.getByText('0 次记录')).toBeVisible()
    expect(screen.getByRole('complementary', { name: '个人记录' })).toBeVisible()
    expect(document.querySelector('main main')).toBeNull()
  })

  it('links a custom item to TMDB without replacing its personal record', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
    let linkRequested = false
    server.use(
      http.get('*/api/v1/media/custom-1', () => HttpResponse.json({
        id: 'custom-1', tmdbId: null, mediaType: 'movie', title: '我的译名', overview: '我的私人简介',
        externalTitle: '', externalOverview: '', originalTitle: '', releaseDate: '2016',
        posterPath: '', backdropPath: '', runtimeMinutes: 0, genres: [],
      })),
      http.get('*/api/v1/records/custom-1', () => HttpResponse.json({
        mediaId: 'custom-1', status: 'wishlist', rating: 8.8, note: '保留这条笔记',
        watchedAt: null, viewingMethod: null, version: 1,
      })),
      http.get('*/api/v1/records/custom-1/events', () => HttpResponse.json([])),
      http.get('*/api/v1/records/custom-1/tags', () => HttpResponse.json({ tags: ['科幻'] })),
      http.get('*/api/v1/household/records/custom-1/sharing', () => HttpResponse.json({
        mediaId: 'custom-1', shareRating: false, shareReview: false, sharedReview: null, version: 1,
      })),
      http.get('*/api/v1/household/participants', () => HttpResponse.json([])),
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
      http.get('*/api/v1/tmdb/search', ({ request }) => {
        expect(new URL(request.url).searchParams.get('q')).toBe('我的译名')
        return HttpResponse.json({
          results: [{
            id: 329865, mediaType: 'movie', title: '降临', originalTitle: 'Arrival',
            year: '2016', posterPath: null,
          }],
        })
      }),
      http.post('*/api/v1/media/custom-1/tmdb/movie/329865', ({ request }) => {
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        linkRequested = true
        return HttpResponse.json({
          id: 'custom-1', tmdbId: 329865, mediaType: 'movie', title: '我的译名', overview: '我的私人简介',
          externalTitle: '降临', externalOverview: '外部简介', originalTitle: 'Arrival', releaseDate: '2016-09-01',
          posterPath: '/arrival.jpg', backdropPath: '', runtimeMinutes: 116, genres: ['科幻'],
        })
      }),
    )
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter initialEntries={['/media/custom-1']}>
        <Routes><Route path="/media/:mediaId" element={<MediaDetailsPage />} /></Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: '我的译名' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: '关联 TMDB' }))
    expect(screen.getByRole('searchbox', { name: '搜索 TMDB 关联' })).toHaveValue('我的译名')
    await user.click(await screen.findByRole('button', { name: '关联 TMDB：降临（2016）' }))

    await waitFor(() => expect(linkRequested).toBe(true))
    expect(screen.getByRole('status')).toHaveTextContent('已关联 TMDB，个人记录保持不变')
    expect(screen.getByText('保留这条笔记')).toBeVisible()
    await user.click(screen.getByText('家庭与整理'))
    expect(screen.getByLabelText('私人标签')).toHaveValue('科幻')
  })

  it('keeps personal recording usable when live TMDB data is unavailable', async () => {
    server.use(
      http.get('*/api/v1/media/media-2', () => HttpResponse.json({
        id: 'media-2', tmdbId: 329865, mediaType: 'movie', title: '本地片名', overview: '本地兜底简介',
        externalTitle: '本地片名', externalOverview: '', originalTitle: 'Local title', releaseDate: '2020',
        posterPath: '', backdropPath: '', runtimeMinutes: 0, genres: [],
      })),
      http.get('*/api/v1/tmdb/movie/329865', () => HttpResponse.json({ code: 'tmdb_unavailable' }, { status: 502 })),
      http.get('*/api/v1/tmdb/movie/329865/credits', () => HttpResponse.json({ code: 'tmdb_unavailable' }, { status: 502 })),
      http.get('*/api/v1/records/media-2', () => HttpResponse.json({
        mediaId: 'media-2', status: 'watching', rating: null, note: '本地笔记',
        watchedAt: null, viewingMethod: null, version: 1,
      })),
      http.get('*/api/v1/records/media-2/events', () => HttpResponse.json([])),
      http.get('*/api/v1/records/media-2/tags', () => HttpResponse.json({ tags: [] })),
      http.get('*/api/v1/household/records/media-2/sharing', () => HttpResponse.json({
        mediaId: 'media-2', shareRating: false, shareReview: false, sharedReview: null, version: 1,
      })),
      http.get('*/api/v1/household/participants', () => HttpResponse.json([])),
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
    )
    renderWithQueryClient(
      <MemoryRouter initialEntries={['/media/media-2']}>
        <Routes><Route path="/media/:mediaId" element={<MediaDetailsPage />} /></Routes>
      </MemoryRouter>,
    )

    expect(await screen.findByRole('heading', { name: '本地片名' })).toBeVisible()
    expect(screen.getByText('本地兜底简介')).toBeVisible()
    expect(screen.getByText('本地笔记')).toBeVisible()
    expect(screen.getByRole('radio', { name: '在看' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('button', { name: '保存记录' })).toBeVisible()
    expect(await screen.findByText('演员资料暂时不可用')).toBeVisible()
  })
})
