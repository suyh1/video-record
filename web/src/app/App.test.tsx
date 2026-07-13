import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { App } from './App'

describe('App', () => {
  it('provides the primary navigation and global search', () => {
    render(<App />)

    expect(screen.getByRole('navigation', { name: '主导航' })).toBeVisible()
    expect(screen.getByRole('searchbox', { name: '搜索影视' })).toBeVisible()
  })
})
