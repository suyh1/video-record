import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { server } from '../test/server'
import { App } from './App'

describe('App', () => {
  beforeEach(() => {
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: true, storageReady: true, tmdbConfigured: false })),
      http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
    )
  })

  it('provides an accessible current page and operable global record action', async () => {
    const user = userEvent.setup()
    render(<App />)

    const navigation = await screen.findByRole('navigation', { name: '主导航' })
    expect(navigation).toBeVisible()
    expect(within(navigation).getByRole('link', { name: '首页' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
    const recordAction = screen.getByRole('button', { name: '记录' })
    expect(recordAction).toBeEnabled()
    await user.click(recordAction)
    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
    expect(document.querySelectorAll('[data-brand-mark="film-archive"]')).toHaveLength(2)
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

  it('routes to the dedicated sync candidate review page', async () => {
    server.use(http.get('*/api/v1/sync/candidates', () => HttpResponse.json([])))
    window.history.pushState({}, '', '/settings/sync')

    render(<App />)

    expect(await screen.findByRole('heading', { name: '同步候选' })).toBeVisible()
    window.history.pushState({}, '', '/')
  })
})
