import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

  it('provides the primary navigation and global search', async () => {
    render(<App />)

    expect(await screen.findByRole('navigation', { name: '主导航' })).toBeVisible()
    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
  })

  it('shows TMDB attribution on the settings page', async () => {
    const currentUserRequest = vi.fn()
    server.use(http.get('*/api/v1/auth/me', () => {
      currentUserRequest()
      return HttpResponse.json({ id: 'member-1', username: 'family', role: 'member' })
    }), http.get('*/api/v1/sync/status', () => HttpResponse.json({ accounts: [], pendingTotal: 0 })))
    window.history.pushState({}, '', '/settings')

    render(<App />)

    expect(await screen.findByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible()
    await waitFor(() => expect(currentUserRequest).toHaveBeenCalledOnce())
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
