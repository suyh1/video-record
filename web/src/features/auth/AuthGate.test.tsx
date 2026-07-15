import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { AuthGate } from './AuthGate'

type DeferredImage = {
  rejectDecode: () => void
  resolveDecode: () => void
  src: string
}

const decodedImages: DeferredImage[] = []

function installDecodedImageMock() {
  class TestImage {
    onerror: ((event: Event) => void) | null = null
    onload: ((event: Event) => void) | null = null
    src = ''
    private rejectPromise!: (reason: Error) => void
    private resolvePromise!: () => void
    private readonly decodePromise = new Promise<void>((resolve, reject) => {
      this.resolvePromise = resolve
      this.rejectPromise = reject
    })
    decode = vi.fn(() => this.decodePromise)

    constructor() {
      decodedImages.push(this)
    }

    resolveDecode = () => this.resolvePromise()
    rejectDecode = () => this.rejectPromise(new Error('decode failed'))
  }

  vi.stubGlobal('Image', TestImage)
}

async function resolveImage(index: number) {
  await act(async () => {
    decodedImages[index]!.resolveDecode()
    await Promise.resolve()
  })
}

async function rejectImage(index: number) {
  await act(async () => {
    decodedImages[index]!.rejectDecode()
    await Promise.resolve()
  })
}

function highlight(id: number, title: string) {
  return {
    id,
    mediaType: id % 2 === 0 ? 'tv' as const : 'movie' as const,
    title,
    originalTitle: `${title} original`,
    year: '2026',
    overview: `${title} overview`,
    backdropURL: `/api/v1/public/tmdb/images/w1280/${id}.jpg?expires=42&signature=signed`,
  }
}

function closedInstanceHandlers() {
  return [
    http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: true, storageReady: true, tmdbConfigured: true })),
    http.get('*/api/v1/auth/me', () => HttpResponse.json({ code: 'unauthenticated' }, { status: 401 })),
  ]
}

describe('AuthGate', () => {
  beforeEach(() => {
    decodedImages.length = 0
    server.use(
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({ items: [] })),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the login form while the independent highlights request is pending', async () => {
    let highlightsRequested = false
    let releaseHighlights!: () => void
    const highlightsPending = new Promise<void>((resolve) => {
      releaseHighlights = resolve
    })
    server.use(
      ...closedInstanceHandlers(),
      http.get('*/api/v1/public/tmdb/highlights', async () => {
        highlightsRequested = true
        await highlightsPending
        return HttpResponse.json({ items: [] })
      }),
    )

    renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    expect(screen.getByLabelText('用户名')).toBeVisible()
    expect(screen.getByLabelText('密码')).toBeVisible()
    await waitFor(() => expect(highlightsRequested).toBe(true))

    releaseHighlights()
  })

  it('activates a decorative same-origin backdrop after the image is decoded', async () => {
    installDecodedImageMock()
    const item = highlight(1, '降临')
    server.use(
      ...closedInstanceHandlers(),
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({ items: [item] })),
    )

    const { container } = renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    await waitFor(() => expect(decodedImages).toHaveLength(1))
    expect(container.querySelector('.auth-page')).toHaveClass('is-empty-backdrop')

    await resolveImage(0)

    const backdrop = container.querySelector<HTMLImageElement>('.auth-backdrop .backdrop-carousel__image.is-active')
    expect(backdrop).toHaveAttribute('src', item.backdropURL)
    expect(backdrop).toHaveAttribute('alt', '')
    expect(backdrop).toHaveAttribute('aria-hidden', 'true')
    expect(container.querySelector('.auth-page')).toHaveClass('has-active-backdrop')
    expect(screen.queryByText('降临')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '上一张背景' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '暂停轮播' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '下一张背景' })).toBeInTheDocument()
  })

  it('rejects external and arbitrary same-origin highlight images before preloading them', async () => {
    installDecodedImageMock()
    const valid = highlight(9, '本站代理图片')
    const unsafeItems = [
      { ...highlight(1, '恶意外域'), backdropURL: 'https://evil.example/a.jpg' },
      { ...highlight(2, 'TMDB 直连'), backdropURL: '//image.tmdb.org/t/p/w1280/direct.jpg' },
      { ...highlight(3, '内联数据'), backdropURL: 'data:image/png;base64,AAAA' },
      { ...highlight(4, '对象地址'), backdropURL: `blob:${window.location.origin}/unsafe` },
      { ...highlight(5, '任意同源接口'), backdropURL: '/api/v1/auth/me' },
      valid,
    ]
    server.use(
      ...closedInstanceHandlers(),
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({ items: unsafeItems })),
    )

    const { container } = renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    await waitFor(() => expect(decodedImages).toHaveLength(1))
    expect(decodedImages[0]!.src).toBe(valid.backdropURL)
    await resolveImage(0)

    expect(container.querySelector('.backdrop-carousel__image.is-active')).toHaveAttribute('src', valid.backdropURL)
    const renderedSources = [...container.querySelectorAll<HTMLImageElement>('img')]
      .map((image) => image.getAttribute('src'))
    for (const unsafe of unsafeItems.slice(0, -1)) {
      expect(decodedImages.some((image) => image.src === unsafe.backdropURL)).toBe(false)
      expect(renderedSources).not.toContain(unsafe.backdropURL)
    }
  })

  it.each([
    ['a highlights server error', () => HttpResponse.json({ code: 'tmdb_unavailable' }, { status: 502 })],
    ['an empty highlights response', () => HttpResponse.json({ items: [] })],
  ])('uses the pure-white empty backdrop state for %s', async (_name, response) => {
    let highlightsRequested = false
    server.use(
      ...closedInstanceHandlers(),
      http.get('*/api/v1/public/tmdb/highlights', () => {
        highlightsRequested = true
        return response()
      }),
    )

    const { container } = renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    await waitFor(() => expect(highlightsRequested).toBe(true))
    await waitFor(() => expect(container.querySelector('.auth-page')).toHaveClass('is-empty-backdrop'))
    expect(container.querySelector('.auth-backdrop img')).toBeNull()
  })

  it('returns to the empty backdrop state when every highlight image fails', async () => {
    installDecodedImageMock()
    server.use(
      ...closedInstanceHandlers(),
      http.get('*/api/v1/public/tmdb/highlights', () => HttpResponse.json({
        items: [highlight(1, '失败一'), highlight(2, '失败二')],
      })),
    )

    const { container } = renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('heading', { name: '登录 video-record' })).toBeVisible()
    await waitFor(() => expect(decodedImages).toHaveLength(1))
    await rejectImage(0)
    await waitFor(() => expect(decodedImages).toHaveLength(2))
    await rejectImage(1)

    expect(container.querySelector('.auth-page')).toHaveClass('is-empty-backdrop')
    expect(container.querySelector('.auth-backdrop img')).toBeNull()
  })

  it('shows and hides the password without changing its value or moving focus', async () => {
    server.use(...closedInstanceHandlers())
    renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    const password = await screen.findByLabelText('密码')
    fireEvent.change(password, { target: { value: 'correct horse battery staple' } })
    password.focus()

    const toggle = screen.getByRole('button', { name: '显示密码' })
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
    fireEvent.click(toggle)
    expect(password).toHaveAttribute('type', 'text')
    expect(password).toHaveValue('correct horse battery staple')
    expect(password).toHaveFocus()
    expect(screen.getByRole('button', { name: '隐藏密码' })).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(screen.getByRole('button', { name: '隐藏密码' }))
    expect(password).toHaveAttribute('type', 'password')
    expect(password).toHaveValue('correct horse battery staple')
    expect(password).toHaveFocus()
  })

  it('keeps the pending login natively disabled and prevents duplicate submissions', async () => {
    let loginRequests = 0
    let releaseLogin!: () => void
    const loginPending = new Promise<void>((resolve) => {
      releaseLogin = resolve
    })
    server.use(
      ...closedInstanceHandlers(),
      http.post('*/api/v1/auth/login', async () => {
        loginRequests += 1
        await loginPending
        return HttpResponse.json({ user: { id: 'admin-1', username: 'owner', role: 'admin' }, csrfToken: 'csrf-after-login' })
      }),
    )
    renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    fireEvent.change(await screen.findByLabelText('用户名'), { target: { value: 'owner' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'correct horse battery staple' } })
    const form = screen.getByRole('button', { name: '登录' }).closest('form')!
    fireEvent.submit(form)
    fireEvent.submit(form)

    await waitFor(() => expect(screen.getByRole('button', { name: '正在登录' })).toBeDisabled())
    expect(loginRequests).toBe(1)
    releaseLogin()
    expect(await screen.findByText('已登录')).toBeVisible()
  })

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

  it('deduplicates pending first-run setup while retaining the administrator fields', async () => {
    let initializeRequests = 0
    let releaseInitialization!: () => void
    const initializationPending = new Promise<void>((resolve) => {
      releaseInitialization = resolve
    })
    server.use(
      http.get('*/api/v1/setup/status', () => HttpResponse.json({ initialized: false, storageReady: true, tmdbConfigured: true })),
      http.post('*/api/v1/setup/admin', async () => {
        initializeRequests += 1
        await initializationPending
        return HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' }, { status: 201 })
      }),
      http.post('*/api/v1/auth/login', () => HttpResponse.json({
        user: { id: 'admin-1', username: 'owner', role: 'admin' },
        csrfToken: 'setup-pending-csrf',
      })),
    )
    renderWithQueryClient(<AuthGate><p>初始化完成</p></AuthGate>)

    const username = await screen.findByLabelText('管理员用户名')
    const password = screen.getByLabelText('管理员密码')
    const confirmation = screen.getByLabelText('确认密码')
    fireEvent.change(username, { target: { value: 'owner' } })
    fireEvent.change(password, { target: { value: 'correct horse battery staple' } })
    fireEvent.change(confirmation, { target: { value: 'correct horse battery staple' } })
    const form = screen.getByRole('button', { name: '创建管理员' }).closest('form')!
    fireEvent.submit(form)
    fireEvent.submit(form)

    const submit = await screen.findByRole('button', { name: '正在创建' })
    expect(submit).toBeDisabled()
    expect(submit).toHaveAttribute('aria-busy', 'true')
    expect(submit).toHaveClass('auth-submit')
    expect(initializeRequests).toBe(1)
    expect(username).toHaveValue('owner')
    expect(password).toHaveValue('correct horse battery staple')
    expect(confirmation).toHaveValue('correct horse battery staple')

    releaseInitialization()
    expect(await screen.findByText('初始化完成')).toBeVisible()
    expect(sessionStorage.getItem('video-record.csrf-token')).toBe('setup-pending-csrf')
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
    expect(screen.getByLabelText('密码')).toHaveValue('wrong password')

    validLogin = true
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'correct horse battery staple' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(screen.getByText('已登录')).toBeVisible())
  })

  it('keeps loading and unavailable states inside the shared authentication scene', async () => {
    let statusAttempts = 0
    let releaseStatus!: () => void
    const pendingStatus = new Promise<void>((resolve) => {
      releaseStatus = resolve
    })
    server.use(
      http.get('*/api/v1/setup/status', async () => {
        statusAttempts += 1
        if (statusAttempts === 1) {
          await pendingStatus
          return HttpResponse.json({ code: 'unavailable' }, { status: 503 })
        }
        return HttpResponse.json({ initialized: false, storageReady: true, tmdbConfigured: false })
      }),
    )

    const { container } = renderWithQueryClient(<AuthGate><p>已登录</p></AuthGate>)

    expect(await screen.findByRole('status')).toHaveTextContent('正在检查实例状态')
    expect(container.querySelector('.auth-scene')).toBeInTheDocument()
    releaseStatus()
    expect(await screen.findByRole('heading', { name: '无法检查实例状态' })).toBeVisible()
    const retry = screen.getByRole('button', { name: '重新检查' })
    expect(retry).toBeEnabled()
    expect(container.querySelector('.auth-scene')).toBeInTheDocument()
    fireEvent.click(retry)
    expect(await screen.findByRole('heading', { name: '开始使用 video-record' })).toBeVisible()
    expect(statusAttempts).toBe(2)
  })
})
