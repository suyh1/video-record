import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { server } from '../test/server'
import { App } from './App'

describe('App', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
    document.documentElement.removeAttribute('data-theme')
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: true, storageReady: true, tmdbConfigured: false })),
      http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/library', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/media/:mediaId', () => HttpResponse.json({ code: 'not_found' }, { status: 404 })),
      http.get('*/api/v1/records/:mediaId', () => HttpResponse.json({ code: 'not_found' }, { status: 404 })),
      http.get('*/api/v1/household/participants', () => HttpResponse.json([])),
      http.get('*/api/v1/sync/candidates', () => HttpResponse.json([])),
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
    )
  })

  it('marks the immersive header for dark text when the home hero is white, including dark theme', async () => {
    document.documentElement.setAttribute('data-theme', 'dark')
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
    const user = userEvent.setup()
    render(<App />)

    const hero = await screen.findByRole('region', { name: '首页主视觉' })
    await waitFor(() => expect(hero).toHaveAttribute('data-backdrop-state', 'empty'))
    expect(screen.getByRole('banner', { name: '应用导航' })).toHaveClass('home-white-header')
    expect(screen.getByRole('heading', { level: 1, name: '首页' })).toBeVisible()

    await user.click(within(screen.getByRole('navigation', { name: '主导航' })).getByRole('link', { name: '影库' }))
    await waitFor(() => expect(window.location.pathname).toBe('/library'))
    expect(screen.getByRole('banner', { name: '应用导航' })).not.toHaveClass('home-white-header', 'home-image-header')
    scrollTo.mockRestore()
  })

  it('places the brand, primary navigation, search, and record action in the application banner', async () => {
    const user = userEvent.setup()
    render(<App />)

    const banner = await screen.findByRole('banner', { name: '应用导航' })
    const navigation = within(banner).getByRole('navigation', { name: '主导航' })
    expect(within(banner).getByRole('link', { name: 'video-record 首页' })).toBeVisible()
    expect(navigation).toBeVisible()
    expect(within(navigation).getByRole('link', { name: '首页' })).toHaveAttribute('aria-current', 'page')
    expect(within(banner).getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
    const recordAction = within(banner).getByRole('button', { name: '记录' })
    expect(recordAction).toBeEnabled()
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
    expect(document.querySelector('.sidebar')).not.toBeInTheDocument()
    await user.click(recordAction)
    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
    expect(document.querySelectorAll('[data-brand-mark="film-archive"]')).toHaveLength(1)
  })

  it('preserves the desktop and mobile navigation names and active page semantics', async () => {
    window.history.replaceState({}, '', '/library')
    render(<App />)

    const primaryNavigation = await screen.findByRole('navigation', { name: '主导航' })
    const mobileNavigation = screen.getByRole('navigation', { name: '移动导航' })
    for (const label of ['首页', '影库', '日历', '统计', '设置']) {
      expect(within(primaryNavigation).getByRole('link', { name: label })).toBeInTheDocument()
      expect(within(mobileNavigation).getByRole('link', { name: label })).toBeInTheDocument()
    }
    expect(within(primaryNavigation).getByRole('link', { name: '影库' })).toHaveAttribute('aria-current', 'page')
    expect(within(mobileNavigation).getByRole('link', { name: '影库' })).toHaveAttribute('aria-current', 'page')
    expect(within(mobileNavigation).getByRole('button', { name: '搜索' })).toBeInTheDocument()
  })

  it('uses a solid header on ordinary routes', async () => {
    window.history.replaceState({}, '', '/settings/sync')
    render(<App />)

    const banner = await screen.findByRole('banner', { name: '应用导航' })
    expect(banner).toHaveClass('app-header', 'solid-header')
    expect(banner).not.toHaveClass('immersive-header', 'is-scrolled')
  })

  it.each(['/', '/media/media-1'])('starts %s with an immersive header and solidifies after scrolling', async (path) => {
    window.history.replaceState({}, '', path)
    render(<App />)

    const banner = await screen.findByRole('banner', { name: '应用导航' })
    expect(banner).toHaveClass('app-header', 'immersive-header')
    expect(banner).not.toHaveClass('is-scrolled')

    Object.defineProperty(window, 'scrollY', { configurable: true, value: 40 })
    act(() => window.dispatchEvent(new Event('scroll')))

    await waitFor(() => expect(banner).toHaveClass('is-scrolled'))
  })

  it('removes the immersive header scroll listener when the shell unmounts', async () => {
    const addEventListener = vi.spyOn(window, 'addEventListener')
    const removeEventListener = vi.spyOn(window, 'removeEventListener')
    const view = render(<App />)
    await screen.findByRole('banner', { name: '应用导航' })

    const scrollRegistrations = addEventListener.mock.calls.filter(([eventName]) => eventName === 'scroll')
    expect(scrollRegistrations).toHaveLength(1)
    const scrollRegistration = scrollRegistrations[0]
    const scrollListener = scrollRegistration?.[1]

    view.unmount()

    expect(removeEventListener).toHaveBeenCalledWith('scroll', scrollListener)
    addEventListener.mockRestore()
    removeEventListener.mockRestore()
  })

  it('resets scroll and immersive header state on PUSH navigation, including between media routes', async () => {
    window.history.replaceState({}, '', '/library')
    server.use(
      http.get('*/api/v1/library', () => HttpResponse.json({ items: [mediaResult('media-a', '第一部')], nextCursor: null })),
      http.get('*/api/v1/media/search', () => HttpResponse.json({ items: [mediaResult('media-b', '第二部')] })),
      http.get('*/api/v1/tmdb/search', () => HttpResponse.json({ results: [] })),
    )
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {
      Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
      window.dispatchEvent(new Event('scroll'))
    })
    const user = userEvent.setup()
    render(<App />)

    Object.defineProperty(window, 'scrollY', { configurable: true, value: 120 })
    await user.click(await screen.findByRole('link', { name: /第一部/ }))

    await waitFor(() => expect(window.location.pathname).toBe('/media/media-a'))
    const banner = screen.getByRole('banner', { name: '应用导航' })
    expect(scrollTo).toHaveBeenCalledWith({ behavior: 'auto', left: 0, top: 0 })
    expect(banner).toHaveClass('immersive-header')
    expect(banner).not.toHaveClass('is-scrolled')

    Object.defineProperty(window, 'scrollY', { configurable: true, value: 96 })
    act(() => window.dispatchEvent(new Event('scroll')))
    await waitFor(() => expect(banner).toHaveClass('is-scrolled'))

    await user.click(screen.getByRole('button', { name: '记录' }))
    await user.type(within(screen.getByRole('dialog', { name: '搜索影视' })).getByRole('searchbox', { name: '搜索影视' }), '第二部')
    await user.click(await screen.findByRole('button', { name: /第二部/ }))

    await waitFor(() => expect(window.location.pathname).toBe('/media/media-b'))
    expect(scrollTo).toHaveBeenCalledTimes(2)
    expect(banner).not.toHaveClass('is-scrolled')
    scrollTo.mockRestore()
  })

  it('preserves restored scroll position on POP navigation', async () => {
    window.history.replaceState({}, '', '/media/pop-source')
    const scrollTo = vi.spyOn(window, 'scrollTo').mockImplementation(() => {
      Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
      window.dispatchEvent(new Event('scroll'))
    })
    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByRole('link', { name: 'video-record 首页' }))
    await waitFor(() => expect(window.location.pathname).toBe('/'))
    scrollTo.mockClear()
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 84 })

    act(() => window.history.back())

    await waitFor(() => expect(window.location.pathname).toBe('/media/pop-source'))
    expect(scrollTo).not.toHaveBeenCalled()
    expect(screen.getByRole('banner', { name: '应用导航' })).toHaveClass('is-scrolled')
    scrollTo.mockRestore()
  })

  it('exposes mobile search selection only while the dialog is open', async () => {
    const user = userEvent.setup()
    render(<App />)

    const mobileSearch = await screen.findByRole('navigation', { name: '移动导航' })
      .then((navigation) => within(navigation).getByRole('button', { name: '搜索' }))
    expect(mobileSearch).toHaveAttribute('aria-expanded', 'false')

    await user.click(mobileSearch)
    expect(mobileSearch).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()

    await user.keyboard('{Escape}')
    await waitFor(() => expect(mobileSearch).toHaveAttribute('aria-expanded', 'false'))
  })

  it('shows TMDB attribution on the settings page', async () => {
    const currentUserRequest = vi.fn()
    server.use(http.get('*/api/v1/auth/me', () => {
      currentUserRequest()
      return HttpResponse.json({ id: 'member-1', username: 'family', role: 'member' })
    }),
    http.get('*/api/v1/sync/status', () => HttpResponse.json({ accounts: [], pendingTotal: 0 })),
    http.get('*/api/v1/integrations/accounts', () => HttpResponse.json([])))
    window.history.pushState({}, '', '/settings')

    render(<App />)

    expect(await screen.findByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible()
    expect(screen.getByText('TMDB 未配置')).toBeVisible()
    await waitFor(() => expect(currentUserRequest).toHaveBeenCalledOnce())
    window.history.pushState({}, '', '/')
  })

  it('links the five settings sections to stable page anchors', async () => {
    server.use(
      http.get('*/api/v1/sync/status', () => HttpResponse.json({ accounts: [], pendingTotal: 0 })),
      http.get('*/api/v1/integrations/accounts', () => HttpResponse.json([])),
      http.get('*/api/v1/household/members', () => HttpResponse.json([])),
      http.get('*/api/v1/backups', () => HttpResponse.json([])),
    )
    window.history.pushState({}, '', '/settings')

    render(<App />)

    const navigation = await screen.findByRole('navigation', { name: '设置章节' })
    const sections = [
      ['账户', '#settings-account'],
      ['TMDB 与媒体服务器', '#settings-connections'],
      ['家庭成员', '#settings-household'],
      ['数据导入导出', '#settings-data'],
      ['备份与恢复', '#settings-backup'],
    ] as const
    expect(within(navigation).getAllByRole('link')).toHaveLength(5)
    for (const [name, href] of sections) {
      expect(within(navigation).getByRole('link', { name })).toHaveAttribute('href', href)
      expect(document.querySelector(href)).toHaveAttribute('id', href.slice(1))
    }
    window.history.pushState({}, '', '/')
  })

  it('shows the current account and logs out from settings', async () => {
    let loggedOut = false
    server.use(
      http.get('*/api/v1/auth/me', () => loggedOut
        ? HttpResponse.json({ code: 'unauthenticated' }, { status: 401 })
        : HttpResponse.json({ id: 'member-1', username: 'family', role: 'member' })),
      http.get('*/api/v1/sync/status', () => HttpResponse.json({ accounts: [], pendingTotal: 0 })),
      http.get('*/api/v1/integrations/accounts', () => HttpResponse.json([])),
      http.post('*/api/v1/auth/logout', () => {
        loggedOut = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    window.history.pushState({}, '', '/settings')
    const user = userEvent.setup()

    render(<App />)

    expect(await screen.findByText('family')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '退出登录' }))
    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    window.history.pushState({}, '', '/')
  })

  it('opens the search dialog when the top searchbox is clicked', async () => {
    render(<App />)

    fireEvent.click(await screen.findByRole('searchbox', { name: '搜索影视' }))

    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
  })

  it('restores focus to the action that opened search', async () => {
    const user = userEvent.setup()
    render(<App />)

    const recordAction = await screen.findByRole('button', { name: '记录' })
    await user.click(recordAction)
    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()

    await user.keyboard('{Escape}')

    await waitFor(() => expect(screen.queryByRole('dialog', { name: '搜索影视' })).not.toBeInTheDocument())
    await waitFor(() => expect(recordAction).toHaveFocus())
  })

  it('routes to the dedicated sync candidate review page', async () => {
    server.use(http.get('*/api/v1/sync/candidates', () => HttpResponse.json([])))
    window.history.pushState({}, '', '/settings/sync')

    render(<App />)

    expect(await screen.findByRole('heading', { name: '同步候选' })).toBeVisible()
    window.history.pushState({}, '', '/')
  })
})

function mediaResult(id: string, title: string) {
  return {
    id,
    source: 'local' as const,
    mediaType: 'movie' as const,
    title,
    originalTitle: '',
    year: '2026',
    posterPath: null,
    status: 'none' as const,
  }
}
