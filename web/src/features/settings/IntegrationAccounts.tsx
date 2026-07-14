import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, CircleCheck, LoaderCircle, Plug, RefreshCw, Server, Trash2, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import {
  APIError,
  createIntegrationAccount,
  disconnectIntegrationAccount,
  getIntegrationAccounts,
} from '../../api/client'
import type { CreateIntegrationAccountPayload, IntegrationAccount, IntegrationProvider } from '../../api/types'

type FormValues = {
  provider: IntegrationProvider
  name: string
  baseUrl: string
  token: string
  userId: string
  accountId: string
  timezone: string
}

const providerLabels: Record<IntegrationProvider, string> = {
  jellyfin: 'Jellyfin',
  emby: 'Emby',
  plex: 'Plex',
}

export function IntegrationAccounts() {
  const queryClient = useQueryClient()
  const [message, setMessage] = useState('')
  const [disconnecting, setDisconnecting] = useState<IntegrationAccount | null>(null)
  const confirmButton = useRef<HTMLButtonElement>(null)
  const accounts = useQuery({
    queryKey: ['integration-accounts'],
    queryFn: ({ signal }) => getIntegrationAccounts(signal),
  })
  const form = useForm<FormValues>({
    defaultValues: {
      provider: 'jellyfin', name: '', baseUrl: '', token: '', userId: '', accountId: '', timezone: '',
    },
  })
  const provider = form.watch('provider')
  const createMutation = useMutation({
    mutationFn: (payload: CreateIntegrationAccountPayload) => createIntegrationAccount(payload),
    onSuccess: (created) => {
      queryClient.setQueryData<IntegrationAccount[]>(['integration-accounts'], (current = []) => [...current, created])
      void queryClient.invalidateQueries({ queryKey: ['sync-status'] })
      form.reset()
      setMessage(`已连接${created.name}`)
    },
    onError: (error) => {
      setMessage(error instanceof APIError && error.code === 'integrations_locked'
        ? '集成密钥不可用。请先配置原 APP_ENCRYPTION_KEY。'
        : '连接失败，请检查地址和字段后重试。当前输入已保留。')
    },
  })
  const disconnectMutation = useMutation({
    mutationFn: (account: IntegrationAccount) => disconnectIntegrationAccount(account.id),
    onSuccess: (_, account) => {
      queryClient.setQueryData<IntegrationAccount[]>(['integration-accounts'], (current = []) => (
        current.filter((item) => item.id !== account.id)
      ))
      void queryClient.invalidateQueries({ queryKey: ['sync-status'] })
      setDisconnecting(null)
      setMessage(`已断开${account.name}`)
    },
    onError: () => {
      setDisconnecting(null)
      setMessage('断开失败，账户保持连接。')
    },
  })

  const submit = form.handleSubmit((values) => {
    const parsed = formSchema.safeParse(values)
    if (!parsed.success) {
      const first = parsed.error.issues[0]
      if (first?.path[0]) form.setError(first.path[0] as keyof FormValues, { message: first.message }, { shouldFocus: true })
      return
    }
    setMessage('')
    createMutation.mutate(toPayload(parsed.data))
  })

  return (
    <section className="integration-accounts" aria-labelledby="integration-accounts-heading">
      <div className="integration-accounts-heading">
        <div>
          <h2 id="integration-accounts-heading">媒体服务器账户</h2>
          <p>凭据提交后立即加密，页面只显示不可逆指纹和运行状态。</p>
        </div>
      </div>

      <form className="integration-account-form" onSubmit={submit} noValidate>
        <label>
          <span>服务类型</span>
          <select {...form.register('provider')}>
            <option value="jellyfin">Jellyfin</option>
            <option value="emby">Emby</option>
            <option value="plex">Plex</option>
          </select>
        </label>
        <label>
          <span>账户名称</span>
          <input maxLength={100} {...form.register('name')} />
          <FieldError message={form.formState.errors.name?.message} />
        </label>
        <label>
          <span>服务器地址</span>
          <input type="url" spellCheck={false} placeholder="https://media.example.test" {...form.register('baseUrl')} />
          <FieldError message={form.formState.errors.baseUrl?.message} />
        </label>
        <label>
          <span>访问令牌</span>
          <input type="password" autoComplete="new-password" maxLength={4096} {...form.register('token')} />
          <FieldError message={form.formState.errors.token?.message} />
        </label>
        {provider === 'plex' ? (
          <label>
            <span>账户 ID</span>
            <input type="number" min="1" inputMode="numeric" {...form.register('accountId')} />
            <FieldError message={form.formState.errors.accountId?.message} />
          </label>
        ) : (
          <label>
            <span>用户 ID</span>
            <input maxLength={256} spellCheck={false} {...form.register('userId')} />
            <FieldError message={form.formState.errors.userId?.message} />
          </label>
        )}
        {provider === 'emby' ? (
          <label>
            <span>服务器时区</span>
            <input placeholder="Asia/Shanghai" spellCheck={false} {...form.register('timezone')} />
            <FieldError message={form.formState.errors.timezone?.message} />
          </label>
        ) : null}
        <button type="submit" disabled={createMutation.isPending}>
          {createMutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Plug aria-hidden="true" size={16} />}
          {createMutation.isPending ? '正在连接' : '连接媒体服务器'}
        </button>
      </form>

      {message ? <p className={createMutation.isError || disconnectMutation.isError ? 'integration-accounts-error' : 'integration-accounts-message'} role={createMutation.isError || disconnectMutation.isError ? 'alert' : 'status'}>{message}</p> : null}
      {accounts.isPending ? <div className="skeleton integration-accounts-skeleton" aria-label="正在加载媒体服务器账户" /> : null}
      {accounts.isError ? (
        <div className="integration-accounts-load-error" role="alert">
          <span>无法读取媒体服务器账户</span>
          <button type="button" onClick={() => void accounts.refetch()}><RefreshCw aria-hidden="true" size={16} />重试</button>
        </div>
      ) : null}
      {accounts.data?.length ? (
        <ul className="integration-account-list" aria-label="已连接媒体服务器账户">
          {accounts.data.map((account) => (
            <li key={account.id}>
              <Server aria-hidden="true" size={18} />
              <div>
                <strong>{account.name}</strong>
                <span>{providerLabels[account.provider]} · 指纹 {account.credentialFingerprint}</span>
              </div>
              <span className={account.locked ? 'integration-account-state locked' : 'integration-account-state connected'}>
                {account.locked ? <AlertTriangle aria-hidden="true" size={16} /> : <CircleCheck aria-hidden="true" size={16} />}
                {account.locked ? '凭据已锁定' : '已连接'}
              </span>
              <button type="button" aria-label={`断开 ${account.name}`} title="断开账户" onClick={() => setDisconnecting(account)}>
                <Trash2 aria-hidden="true" size={16} />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      {accounts.data?.length === 0 ? <p className="integration-accounts-empty">还没有媒体服务器账户。</p> : null}

      <Dialog.Root open={Boolean(disconnecting)} onOpenChange={(open) => { if (!open) setDisconnecting(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-backdrop" />
          <Dialog.Content
            className="member-dialog"
            onOpenAutoFocus={(event) => { event.preventDefault(); confirmButton.current?.focus() }}
          >
            <Dialog.Title>断开媒体服务器</Dialog.Title>
            <Dialog.Description>
              断开 {disconnecting?.name ?? ''} 会删除其同步游标、映射和待核对候选，已有观影记录不会删除。
            </Dialog.Description>
            <div className="dialog-actions">
              <Dialog.Close asChild><button type="button"><X aria-hidden="true" size={16} />取消</button></Dialog.Close>
              <button
                ref={confirmButton}
                type="button"
                disabled={!disconnecting || disconnectMutation.isPending}
                onClick={() => { if (disconnecting) disconnectMutation.mutate(disconnecting) }}
              >
                {disconnectMutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Trash2 aria-hidden="true" size={16} />}
                {disconnectMutation.isPending ? '正在断开' : '确认断开'}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </section>
  )
}

function FieldError({ message }: { message: string | undefined }) {
  return message ? <small>{message}</small> : null
}

const formSchema = z.object({
  provider: z.enum(['jellyfin', 'emby', 'plex']),
  name: z.string().trim().min(1, '请输入账户名称').max(100, '账户名称不能超过 100 字'),
  baseUrl: z.string().trim().url('请输入有效的 http 或 https 地址').refine((value) => /^https?:\/\//i.test(value), '只支持 http 或 https 地址'),
  token: z.string().trim().min(1, '请输入访问令牌').max(4096, '访问令牌过长'),
  userId: z.string().trim(),
  accountId: z.string().trim(),
  timezone: z.string().trim(),
}).superRefine((values, context) => {
  if (values.provider !== 'plex' && !values.userId) {
    context.addIssue({ code: 'custom', path: ['userId'], message: '请输入用户 ID' })
  }
  if (values.provider === 'plex' && (!/^\d+$/.test(values.accountId) || Number(values.accountId) < 1)) {
    context.addIssue({ code: 'custom', path: ['accountId'], message: '请输入正整数账户 ID' })
  }
})

function toPayload(values: FormValues): CreateIntegrationAccountPayload {
  const payload: CreateIntegrationAccountPayload = {
    provider: values.provider,
    name: values.name.trim(),
    baseUrl: values.baseUrl.trim(),
    token: values.token.trim(),
  }
  if (values.provider === 'plex') payload.accountId = Number(values.accountId)
  else payload.userId = values.userId.trim()
  if (values.provider === 'emby' && values.timezone.trim()) payload.timezone = values.timezone.trim()
  return payload
}
