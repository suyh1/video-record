import { fireEvent, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { AuthGate } from './AuthGate'

describe('AuthGate', () => {
  it('closes first-run setup by creating and signing in the administrator', async () => {
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: false, storageReady: true, tmdbConfigured: false })),
      http.post('*/api/v1/setup/admin', async ({ request }) => {
        const body = await request.json() as { username: string }
        return HttpResponse.json({ id: 'admin-1', username: body.username, role: 'admin' }, { status: 201 })
      }),
      http.post('*/api/v1/auth/login', () => HttpResponse.json({
        user: { id: 'admin-1', username: 'owner', role: 'admin' },
        csrfToken: 'synthetic-csrf-token',
      })),
    )
    renderWithQueryClient(<AuthGate><p>私人影库已打开</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '开始使用 video-record' })).toBeVisible()
    expect(document.querySelector('[data-brand-mark="film-archive"]')).toBeInTheDocument()
    expect(screen.getByText('数据存储已就绪')).toBeVisible()
    expect(screen.getByText('TMDB 尚未配置')).toBeVisible()

    fireEvent.change(screen.getByLabelText('管理员用户名'), { target: { value: 'owner' } })
    fireEvent.change(screen.getByLabelText('管理员密码'), { target: { value: 'correct horse battery staple' } })
    fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'different password value' } })
    fireEvent.click(screen.getByRole('button', { name: '创建管理员' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('两次输入的密码不一致')

    fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'correct horse battery staple' } })
    fireEvent.click(screen.getByRole('button', { name: '创建管理员' }))
    expect(await screen.findByText('私人影库已打开')).toBeVisible()
    expect(sessionStorage.getItem('video-record.csrf-token')).toBe('synthetic-csrf-token')
  })

  it('shows a closed login and preserves the username after invalid credentials', async () => {
    let validLogin = false
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: true, storageReady: true, tmdbConfigured: true })),
      http.get('*/api/v1/auth/me', () => HttpResponse.json({ code: 'unauthenticated' }, { status: 401 })),
      http.post('*/api/v1/auth/login', async () => validLogin
        ? HttpResponse.json({ user: { id: 'admin-1', username: 'owner', role: 'admin' }, csrfToken: 'csrf-after-login' })
        : HttpResponse.json({ code: 'invalid_credentials', requestId: 'request-1' }, { status: 401 })),
    )
    renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'owner' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'wrong password' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('用户名或密码不正确')
    expect(screen.getByLabelText('用户名')).toHaveValue('owner')

    validLogin = true
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'correct horse battery staple' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(screen.getByText('已登录')).toBeVisible())
  })
})
