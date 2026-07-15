import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { server } from '../test/server'
import { App } from './App'

describe('App', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
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
    )
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
