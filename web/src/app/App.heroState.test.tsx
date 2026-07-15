import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { HomeHeroBackdropState } from '../features/home/HomeHero'
import { server } from '../test/server'
import { App } from './App'

const homeMounts = vi.hoisted(() => ({ count: 0 }))

vi.mock('../features/home/HomePage', () => ({
  HomePage({ onHeroBackdropStateChange }: {
    onHeroBackdropStateChange?: (state: HomeHeroBackdropState) => void
  }) {
    useEffect(() => {
      homeMounts.count += 1
      if (homeMounts.count === 1) onHeroBackdropStateChange?.('ready')
    }, [onHeroBackdropStateChange])

    return <section aria-label="首页主视觉" data-mount={homeMounts.count + 1} />
  },
}))

describe('App home hero header state', () => {
  beforeEach(() => {
    homeMounts.count = 0
    window.history.replaceState({}, '', '/')
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: true, storageReady: true, tmdbConfigured: true })),
      http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({ items: [] })),
      http.get('*/api/v1/library', () => HttpResponse.json({ items: [], nextCursor: null })),
      http.get('*/api/v1/collections', () => HttpResponse.json([])),
    )
  })

  afterEach(() => vi.restoreAllMocks())

  it('resets a ready home hero before returning to a still-pending home', async () => {
    const user = userEvent.setup()
    render(<App />)

    const navigation = await screen.findByRole('navigation', { name: '主导航' })
    const banner = screen.getByRole('banner', { name: '应用导航' })
    await waitFor(() => expect(banner).toHaveClass('home-image-header'))

    await user.click(within(navigation).getByRole('link', { name: '影库' }))
    await waitFor(() => expect(window.location.pathname).toBe('/library'))
    await user.click(within(navigation).getByRole('link', { name: '首页' }))

    await waitFor(() => expect(window.location.pathname).toBe('/'))
    expect(await screen.findByRole('region', { name: '首页主视觉' })).toHaveAttribute('data-mount', '2')
    expect(banner).toHaveClass('home-white-header')
    expect(banner).not.toHaveClass('home-image-header')
  })
})
