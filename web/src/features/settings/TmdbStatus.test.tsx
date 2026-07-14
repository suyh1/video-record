import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { TmdbStatus } from './TmdbStatus'

describe('TmdbStatus', () => {
  it('shows configured status and required attribution without credential fields', () => {
    renderWithQueryClient(<TmdbStatus configured />)

    expect(screen.getByText('TMDB 已配置')).toBeVisible()
    expect(screen.getByRole('button', { name: '测试连通' })).toBeVisible()
    expect(screen.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible()
    expect(screen.getByRole('link', { name: '访问 TMDB' })).toHaveAttribute('href', 'https://www.themoviedb.org/')
    expect(screen.queryByLabelText('TMDB 令牌')).not.toBeInTheDocument()
  })

  it('shows unconfigured status with a non-color warning label', () => {
    renderWithQueryClient(<TmdbStatus configured={false} />)

    expect(screen.getByText('TMDB 未配置')).toBeVisible()
    expect(screen.getByText('需要由服务端设置环境变量 TMDB_READ_ACCESS_TOKEN')).toBeVisible()
    expect(screen.queryByRole('button', { name: '测试连通' })).not.toBeInTheDocument()
  })

  it('disables repeat tests while pending and announces success', async () => {
    let releaseRequest: () => void = () => {}
    server.use(http.get('*/api/v1/tmdb/connectivity', async () => {
      await new Promise<void>((resolve) => { releaseRequest = resolve })
      return HttpResponse.json({ connected: true })
    }))
    const user = userEvent.setup()
    renderWithQueryClient(<TmdbStatus configured />)

    await user.click(screen.getByRole('button', { name: '测试连通' }))

    expect(screen.getByRole('button', { name: '正在测试' })).toBeDisabled()
    releaseRequest()
    const success = await screen.findByText('TMDB 连通正常')
    expect(success).toHaveAttribute('role', 'status')
  })

  it.each([
    ['tmdb_unauthorized', 502, 'TMDB 令牌无效，请检查服务端配置。'],
    ['tmdb_timeout', 504, '连接 TMDB 超时，请检查服务端代理或网络设置。'],
    ['tmdb_rate_limited', 503, 'TMDB 请求受限，请稍后重试。'],
    ['tmdb_unavailable', 502, '无法连接 TMDB，请检查服务端代理或网络设置。'],
  ])('announces %s failures with actionable guidance', async (code, status, message) => {
    server.use(http.get('*/api/v1/tmdb/connectivity', () => HttpResponse.json({ code }, { status })))
    const user = userEvent.setup()
    renderWithQueryClient(<TmdbStatus configured />)

    await user.click(screen.getByRole('button', { name: '测试连通' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(message)
  })
})
