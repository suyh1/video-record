import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CircleAlert, CircleCheck, Clapperboard, Database, LoaderCircle, LogIn, UserRoundPlus } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'

import {
  APIError,
  getCurrentUser,
  getSetupStatus,
  initializeAdministrator,
  loginUser,
} from '../../api/client'
import type { CurrentUser, SetupStatus } from '../../api/types'

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

  if (setup.isPending || (setup.data?.initialized && currentUser.isPending)) return <AuthLoading />
  if (setup.isError) {
    return <AuthUnavailable onRetry={() => void setup.refetch()} />
  }
  if (!setup.data.initialized) return <SetupPage status={setup.data} onAuthenticated={authenticated} />
  if (currentUser.isError) return <LoginPage onAuthenticated={authenticated} />
  return children
}

function SetupPage({ status, onAuthenticated }: { status: SetupStatus; onAuthenticated: (user: CurrentUser) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (password !== confirmation) {
      setError('两次输入的密码不一致')
      return
    }
    setError('')
    setPending(true)
    try {
      await initializeAdministrator(username, password)
      const session = await loginUser(username, password)
      sessionStorage.setItem('video-record.csrf-token', session.csrfToken)
      onAuthenticated(session.user)
    } catch (caught) {
      setError(setupError(caught))
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-panel" aria-labelledby="setup-heading">
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
          <label>
            <span>管理员用户名</span>
            <input autoComplete="username" minLength={3} required value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label>
            <span>管理员密码</span>
            <input autoComplete="new-password" minLength={12} required type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
          </label>
          <label>
            <span>确认密码</span>
            <input autoComplete="new-password" minLength={12} required type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />
          </label>
          {error ? <p className="auth-error" role="alert">{error}</p> : null}
          <button className="primary-button auth-submit" type="submit" disabled={pending || !status.storageReady}>
            {pending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={17} /> : <UserRoundPlus aria-hidden="true" size={17} />}
            {pending ? '正在创建' : '创建管理员'}
          </button>
        </form>
      </section>
    </main>
  )
}

function LoginPage({ onAuthenticated }: { onAuthenticated: (user: CurrentUser) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setPending(true)
    try {
      const session = await loginUser(username, password)
      sessionStorage.setItem('video-record.csrf-token', session.csrfToken)
      onAuthenticated(session.user)
    } catch (caught) {
      setError(loginError(caught))
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="auth-page">
      <section className="auth-panel login-panel" aria-labelledby="login-heading">
        <AuthBrand />
        <div className="auth-heading">
          <p>私人影库</p>
          <h1 id="login-heading">登录 video-record</h1>
          <p>此实例不开放注册。家庭成员账户由管理员创建。</p>
        </div>
        <form className="auth-form" onSubmit={(event) => void submit(event)} noValidate>
          <label>
            <span>用户名</span>
            <input autoComplete="username" required value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label>
            <span>密码</span>
            <input autoComplete="current-password" required type="password" value={password} onChange={(event) => setPassword(event.target.value)} />
          </label>
          {error ? <p className="auth-error" role="alert">{error}</p> : null}
          <button className="primary-button auth-submit" type="submit" disabled={pending}>
            {pending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={17} /> : <LogIn aria-hidden="true" size={17} />}
            {pending ? '正在登录' : '登录'}
          </button>
        </form>
      </section>
    </main>
  )
}

function AuthBrand() {
  return (
    <div className="auth-brand" aria-label="video-record">
      <Clapperboard aria-hidden="true" size={22} strokeWidth={1.8} />
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
    <main className="auth-page">
      <p className="auth-loading" role="status"><LoaderCircle className="loading-icon" aria-hidden="true" size={18} />正在检查实例状态</p>
    </main>
  )
}

function AuthUnavailable({ onRetry }: { onRetry: () => void }) {
  return (
    <main className="auth-page">
      <section className="auth-panel login-panel" aria-labelledby="unavailable-heading">
        <AuthBrand />
        <div className="auth-heading">
          <p>连接中断</p>
          <h1 id="unavailable-heading">无法检查实例状态</h1>
          <p role="alert">服务器暂时不可用，当前页面没有提交任何凭据。</p>
        </div>
        <button className="primary-button auth-submit" type="button" onClick={onRetry}>重新检查</button>
      </section>
    </main>
  )
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
