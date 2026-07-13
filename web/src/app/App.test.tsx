import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { App } from './App'

describe('App', () => {
  it('provides the primary navigation and global search', () => {
    render(<App />)

    expect(screen.getByRole('navigation', { name: '主导航' })).toBeVisible()
    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
  })

  it('shows TMDB attribution on the settings page', () => {
    window.history.pushState({}, '', '/settings')

    render(<App />)

    expect(screen.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible()
    window.history.pushState({}, '', '/')
  })

  it('opens the search dialog when the top searchbox is clicked', () => {
    render(<App />)

    fireEvent.click(screen.getByRole('searchbox', { name: '搜索影视' }))

    expect(screen.getByRole('dialog', { name: '搜索影视' })).toBeVisible()
  })
})
