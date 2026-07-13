import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'

import { server } from '../test/server'
import { App } from './App'

describe('App', () => {
  it('provides the primary navigation and global search', () => {
    render(<App />)

    expect(screen.getByRole('navigation', { name: '主导航' })).toBeVisible()
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

    expect(screen.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible()
    await waitFor(() => expect(currentUserRequest).toHaveBeenCalledOnce())
    window.history.pushState({}, '', '/')
  })

  it('opens the search dialog when the top searchbox is clicked', () => {
    render(<App />)

    fireEvent.click(screen.getByRole('searchbox', { name: '搜索影视' }))

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
