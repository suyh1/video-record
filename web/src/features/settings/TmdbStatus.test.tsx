import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { TmdbStatus } from './TmdbStatus'

describe('TmdbStatus', () => {
  it('shows configured status and required attribution without credential fields', () => {
    render(<TmdbStatus configured />)

    expect(screen.getByText('TMDB 已配置')).toBeVisible()
    expect(screen.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible()
    expect(screen.getByRole('link', { name: '访问 TMDB' })).toHaveAttribute('href', 'https://www.themoviedb.org/')
    expect(screen.queryByLabelText('TMDB 令牌')).not.toBeInTheDocument()
  })

  it('shows unconfigured status with a non-color warning label', () => {
    render(<TmdbStatus configured={false} />)

    expect(screen.getByText('TMDB 未配置')).toBeVisible()
    expect(screen.getByText('需要由服务端设置环境变量 TMDB_READ_ACCESS_TOKEN')).toBeVisible()
  })
})
