import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CircleAlert,
  CircleCheck,
  Database,
  Eye,
  EyeOff,
  LoaderCircle,
  LockKeyhole,
  LogIn,
  UserRound,
  UserRoundPlus,
} from 'lucide-react'
import { type FormEvent, type MouseEvent, type ReactNode, useCallback, useMemo, useRef, useState } from 'react'

import {
  APIError,
  getCurrentUser,
  getSetupStatus,
  getTMDBHighlights,
  initializeAdministrator,
  loginUser,
} from '../../api/client'
import type { CurrentUser, SetupStatus, TMDBHighlight } from '../../api/types'
import { BrandMark } from '../../app/BrandMark'
import { BackdropCarousel } from '../highlights/BackdropCarousel'

export function AuthGate({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const setup = useQuery({
    queryKey: ['setup-status'],
    queryFn: ({ signal }) => getSetupStatus(signal),
    retry: false,
  })
  const currentUser = useQuery({
    queryKey: ['current-user'],
    queryFn: ({ signal }) => getCurrentUser(signal),
    enabled: setup.data?.initialized === true,
    retry: false,
    staleTime: 30_000,
  })

  const authenticated = (user: CurrentUser) => {
    queryClient.setQueryData(['setup-status'], (status: SetupStatus | undefined) => ({
      initialized: true,
      storageReady: status?.storageReady ?? true,
      tmdbConfigured: status?.tmdbConfigured ?? false,
    }))
    queryClient.setQueryData(['current-user'], user)
  }

  if (setup.isPending || (setup.data?.initialized && currentUser.isPending)) {
    return <AuthScene><AuthLoading /></AuthScene>
  }
  if (setup.isError) {
    return <AuthScene><AuthUnavailable onRetry={() => void setup.refetch()} /></AuthScene>
  }
  if (!setup.data.initialized) {
    return <AuthScene><SetupPage status={setup.data} onAuthenticated={authenticated} /></AuthScene>
  }
  if (currentUser.isError) {
    return <AuthScene><LoginPage onAuthenticated={authenticated} /></AuthScene>
  }
  return children
}

function AuthScene({ children }: { children: ReactNode }) {
  const highlights = useQuery({
    queryKey: ['tmdb-highlights'],
    queryFn: ({ signal }) => getTMDBHighlights(signal),
    retry: false,
  })
  const items = useMemo(
    () => (highlights.data ?? []).filter((item) => isSameOriginTMDBImage(item.backdropURL)),
    [highlights.data],
  )
  const [hasActiveBackdrop, setHasActiveBackdrop] = useState(false)
  const handleActiveItemChange = useCallback((item: TMDBHighlight | null) => {
    setHasActiveBackdrop(item !== null)
  }, [])

  return (
    <main className={`auth-page auth-scene ${hasActiveBackdrop ? 'has-active-backdrop' : 'is-empty-backdrop'}`}>
      <div className="auth-backdrop">
        <BackdropCarousel
          intervalMs={7_000}
          items={items}
          onActiveItemChange={handleActiveItemChange}
          showControls
        />
      </div>
      <div className="auth-scrim" aria-hidden="true" />
      <div className="auth-scene-content">{children}</div>
    </main>
  )
}

function SetupPage({ status, onAuthenticated }: { status: SetupStatus; onAuthenticated: (user: CurrentUser) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const requestPending = useRef(false)

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (requestPending.current) return
    if (password !== confirmation) {
      setError('两次输入的密码不一致')
      return
    }
    setError('')
    requestPending.current = true
    setPending(true)
    try {
      await initializeAdministrator(username, password)
      const session = await loginUser(username, password)
      sessionStorage.setItem('video-record.csrf-token', session.csrfToken)
      onAuthenticated(session.user)
    } catch (caught) {
      setError(setupError(caught))
    } finally {
      requestPending.current = false
      setPending(false)
    }
  }

  return (
    <section className="auth-panel setup-panel" aria-labelledby="setup-heading">
      <AuthBrand />
      <div className="auth-heading">
        <p>封闭初始化</p>
        <h1 id="setup-heading">开始使用 video-record</h1>
        <p>创建首位管理员后，公开初始化入口将立即关闭。</p>
      </div>
      <ul className="setup-status" aria-label="服务状态">
        <StatusItem ready={status.storageReady} icon={Database} readyText="数据存储已就绪" unavailableText="数据存储不可用" />
        <StatusItem ready={status.tmdbConfigured} icon={status.tmdbConfigured ? CircleCheck : CircleAlert} readyText="TMDB 已配置" unavailableText="TMDB 尚未配置" />
      </ul>
      <form className="auth-form" onSubmit={(event) => void submit(event)} noValidate>
        <AuthTextField
          autoComplete="username"
          icon={UserRound}
          id="setup-username"
          label="管理员用户名"
          minLength={3}
          onChange={setUsername}
          required
          value={username}
        />
        <PasswordField
          autoComplete="new-password"
          id="setup-password"
          label="管理员密码"
          minLength={12}
          onChange={setPassword}
          toggleLabel="管理员密码"
          value={password}
        />
        <PasswordField
          autoComplete="new-password"
          id="setup-password-confirmation"
          label="确认密码"
          minLength={12}
          onChange={setConfirmation}
          toggleLabel="确认密码"
          value={confirmation}
        />
        {error ? <p className="auth-error" role="alert">{error}</p> : null}
        <button
          aria-busy={pending}
          className="primary-button auth-submit"
          type="submit"
          disabled={pending || !status.storageReady}
        >
          {pending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={17} /> : <UserRoundPlus aria-hidden="true" size={17} />}
          {pending ? '正在创建' : '创建管理员'}
        </button>
      </form>
    </section>
  )
}

function LoginPage({ onAuthenticated }: { onAuthenticated: (user: CurrentUser) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const requestPending = useRef(false)

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (requestPending.current) return
    setError('')
    requestPending.current = true
    setPending(true)
    try {
      const session = await loginUser(username, password)
      sessionStorage.setItem('video-record.csrf-token', session.csrfToken)
      onAuthenticated(session.user)
    } catch (caught) {
      setError(loginError(caught))
    } finally {
      requestPending.current = false
      setPending(false)
    }
  }

  return (
    <section className="auth-panel login-panel" aria-labelledby="login-heading">
      <AuthBrand />
      <div className="auth-heading">
        <p>欢迎回来</p>
        <h1 id="login-heading">登录 video-record</h1>
        <p>此实例不开放注册。家庭成员账户由管理员创建。</p>
      </div>
      <form className="auth-form" onSubmit={(event) => void submit(event)} noValidate>
        <AuthTextField
          autoComplete="username"
          icon={UserRound}
          id="login-username"
          label="用户名"
          onChange={setUsername}
          required
          value={username}
        />
        <PasswordField
          autoComplete="current-password"
          id="login-password"
          label="密码"
          onChange={setPassword}
          toggleLabel="密码"
          value={password}
        />
        {error ? <p className="auth-error" role="alert">{error}</p> : null}
        <button aria-busy={pending} className="primary-button auth-submit" type="submit" disabled={pending}>
          {pending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={17} /> : <LogIn aria-hidden="true" size={17} />}
          {pending ? '正在登录' : '登录'}
        </button>
      </form>
    </section>
  )
}

function AuthTextField({
  autoComplete,
  icon: Icon,
  id,
  label,
  minLength,
  onChange,
  required = false,
  value,
}: {
  autoComplete: string
  icon: typeof UserRound
  id: string
  label: string
  minLength?: number
  onChange: (value: string) => void
  required?: boolean
  value: string
}) {
  return (
    <div className="auth-field">
      <label htmlFor={id}>{label}</label>
      <div className="auth-input-shell">
        <Icon aria-hidden="true" size={18} />
        <input
          autoComplete={autoComplete}
          id={id}
          minLength={minLength}
          onChange={(event) => onChange(event.target.value)}
          required={required}
          value={value}
        />
      </div>
    </div>
  )
}

function PasswordField({ autoComplete, id, label, minLength, onChange, toggleLabel, value }: {
  autoComplete: string
  id: string
  label: string
  minLength?: number
  onChange: (value: string) => void
  toggleLabel: string
  value: string
}) {
  const [visible, setVisible] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const actionLabel = visible ? '隐藏' : '显示'
  const accessibleLabel = `${actionLabel}${toggleLabel}`

  const toggleVisibility = (event: MouseEvent<HTMLButtonElement>) => {
    setVisible((current) => !current)
    if (event.detail > 0) inputRef.current?.focus()
  }

  return (
    <div className="auth-field">
      <label htmlFor={id}>{label}</label>
      <div className="auth-input-shell auth-password-shell">
        <LockKeyhole aria-hidden="true" size={18} />
        <input
          autoComplete={autoComplete}
          id={id}
          minLength={minLength}
          onChange={(event) => onChange(event.target.value)}
          ref={inputRef}
          required
          type={visible ? 'text' : 'password'}
          value={value}
        />
        <button
          aria-label={accessibleLabel}
          aria-pressed={visible}
          className="auth-password-toggle"
          onClick={toggleVisibility}
          onMouseDown={(event) => event.preventDefault()}
          title={accessibleLabel}
          type="button"
        >
          {visible ? <EyeOff aria-hidden="true" size={19} /> : <Eye aria-hidden="true" size={19} />}
        </button>
      </div>
    </div>
  )
}

function AuthBrand() {
  return (
    <div className="auth-brand" aria-label="video-record">
      <BrandMark size={22} />
      <span>video-record</span>
    </div>
  )
}

function StatusItem({ ready, icon: Icon, readyText, unavailableText }: {
  ready: boolean
  icon: typeof Database
  readyText: string
  unavailableText: string
}) {
  return (
    <li className={ready ? 'ready' : 'notice'}>
      <Icon aria-hidden="true" size={17} />
      <span>{ready ? readyText : unavailableText}</span>
    </li>
  )
}

function AuthLoading() {
  return (
    <section className="auth-panel login-panel auth-status-panel" aria-labelledby="loading-heading">
      <AuthBrand />
      <div className="auth-heading">
        <p>实例连接</p>
        <h1 id="loading-heading">video-record</h1>
        <p className="auth-loading" role="status"><LoaderCircle className="loading-icon" aria-hidden="true" size={18} />正在检查实例状态</p>
      </div>
    </section>
  )
}

function AuthUnavailable({ onRetry }: { onRetry: () => void }) {
  return (
    <section className="auth-panel login-panel auth-status-panel" aria-labelledby="unavailable-heading">
      <AuthBrand />
      <div className="auth-heading">
        <p>连接中断</p>
        <h1 id="unavailable-heading">无法检查实例状态</h1>
        <p role="alert">服务器暂时不可用，当前页面没有提交任何凭据。</p>
      </div>
      <button className="primary-button auth-submit" type="button" onClick={onRetry}>重新检查</button>
    </section>
  )
}

function isSameOriginTMDBImage(value: string) {
  try {
    const imageURL = new URL(value, window.location.origin)
    return imageURL.origin === window.location.origin
      && imageURL.pathname.startsWith('/api/v1/public/tmdb/images/')
  } catch {
    return false
  }
}

function setupError(error: unknown) {
  if (error instanceof APIError && error.code === 'initialization_closed') return '初始化已经关闭，请刷新页面后登录。'
  if (error instanceof APIError && error.code === 'invalid_input') return '用户名至少 3 个字符，密码至少 12 个字符。'
  return '无法创建管理员，请检查连接后重试。'
}

function loginError(error: unknown) {
  if (error instanceof APIError && error.code === 'invalid_credentials') return '用户名或密码不正确'
  if (error instanceof APIError && error.code === 'login_rate_limited') return '登录尝试过多，请稍后再试。'
  return '无法登录，请检查连接后重试。'
}
