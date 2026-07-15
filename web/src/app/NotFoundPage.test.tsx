import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'

import { server } from '../test/server'
import { App } from './App'

describe('NotFoundPage route', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/missing-archive')
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: true, storageReady: true, tmdbConfigured: false })),
      http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({ items: [] })),
    )
  })

  it('offers a real home link from an unknown route', async () => {
    render(<App />)

    expect(await screen.findByRole('heading', { name: '没有找到这份档案' })).toBeVisible()
    expect(screen.getByRole('link', { name: '返回首页' })).toHaveAttribute('href', '/')
  })

  it('opens the existing search dialog from the recovery action', async () => {
    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByRole('button', { name: '搜索影视' }))

    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
  })
})
