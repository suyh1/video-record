import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { TMDBCastMember } from '../../api/types'
import { CastStrip } from './CastStrip'

const signature = 'c'.repeat(64)

function member(overrides: Partial<TMDBCastMember> = {}): TMDBCastMember {
  return {
    id: 1,
    name: '林见川',
    character: '顾潮',
    profilePath: `/api/v1/public/tmdb/images/w300/cast-one.png?expires=1784200000&signature=${signature}`,
    order: 0,
    ...overrides,
  }
}

function renderCast(cast: TMDBCastMember[]) {
  return render(
    <CastStrip cast={cast} pending={false} error={false} linked onRetry={() => undefined} />,
  )
}

describe('CastStrip portraits', () => {
  it('keeps a signed same-origin portrait URL unchanged with an accurate accessible name', () => {
    renderCast([member()])

    expect(screen.getByRole('img', { name: '林见川 饰 顾潮' })).toHaveAttribute('src', member().profilePath)
  })

  it('replaces a failed portrait with a named initial placeholder', () => {
    renderCast([member()])

    fireEvent.error(screen.getByRole('img', { name: '林见川 饰 顾潮' }))

    expect(screen.queryByRole('img', { name: '林见川 饰 顾潮' })).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: '林见川 饰 顾潮 暂无头像' })).toHaveTextContent('林')
  })

  it.each([
    '/raw-cast.jpg',
    '/api/v1/public/tmdb/images/w300/unsigned.jpg',
    'https://image.tmdb.org/t/p/w300/direct.jpg',
  ])('treats an unsafe portrait source as missing: %s', (profilePath) => {
    renderCast([member({ profilePath })])

    expect(screen.queryByRole('img', { name: '林见川 饰 顾潮' })).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: '林见川 饰 顾潮 暂无头像' })).toBeVisible()
  })

  it('retries after the same cast item receives a new portrait or name', () => {
    const restoredURL = `/api/v1/public/tmdb/images/w300/restored.png?expires=1784200000&signature=${signature}`
    const { rerender } = renderCast([member()])
    fireEvent.error(screen.getByRole('img', { name: '林见川 饰 顾潮' }))

    rerender(<CastStrip cast={[member({ profilePath: restoredURL })]} pending={false} error={false} linked onRetry={() => undefined} />)
    expect(screen.getByRole('img', { name: '林见川 饰 顾潮' })).toHaveAttribute('src', restoredURL)

    fireEvent.error(screen.getByRole('img', { name: '林见川 饰 顾潮' }))
    rerender(<CastStrip cast={[member({ name: '周远', profilePath: restoredURL })]} pending={false} error={false} linked onRetry={() => undefined} />)
    expect(screen.getByRole('img', { name: '周远 饰 顾潮' })).toHaveAttribute('src', restoredURL)
  })

  it('keeps duplicate credits independent without duplicate React keys', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const secondProfile = `/api/v1/public/tmdb/images/w300/cast-two.png?expires=1784200000&signature=${signature}`
    try {
      renderCast([
        member(),
        member({ name: '周聆', profilePath: secondProfile, order: 1 }),
      ])

      const first = screen.getByRole('img', { name: '林见川 饰 顾潮' })
      const second = screen.getByRole('img', { name: '周聆 饰 顾潮' })
      fireEvent.error(first)
      fireEvent.error(second)

      expect(screen.getByRole('img', { name: '林见川 饰 顾潮 暂无头像' })).toHaveTextContent('林')
      expect(screen.getByRole('img', { name: '周聆 饰 顾潮 暂无头像' })).toHaveTextContent('周')
      expect(consoleError.mock.calls.flat().join(' ')).not.toContain('same key')
    } finally {
      consoleError.mockRestore()
    }
  })
})
