import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, KeyRound, LoaderCircle, Plus, Shield, UserRound, UserX, X } from 'lucide-react'
import { useRef, useState } from 'react'

import {
  createHouseholdMember,
  deactivateHouseholdMember,
  getCurrentUser,
  getHouseholdMembers,
  resetHouseholdMemberPassword,
} from '../../api/client'
import type { HouseholdMember } from '../../api/types'

type MemberAction = { type: 'deactivate' | 'reset'; member: HouseholdMember } | null

export function MemberSettings() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [action, setAction] = useState<MemberAction>(null)
  const [replacementPassword, setReplacementPassword] = useState('')
  const [message, setMessage] = useState('')
  const confirmButton = useRef<HTMLButtonElement>(null)
  const currentUser = useQuery({ queryKey: ['current-user'], queryFn: ({ signal }) => getCurrentUser(signal) })
  const members = useQuery({
    queryKey: ['household-members'],
    queryFn: ({ signal }) => getHouseholdMembers(signal),
    enabled: currentUser.data?.role === 'admin',
  })
  const createMutation = useMutation({
    mutationFn: () => createHouseholdMember(username, password),
    onSuccess: () => {
      setUsername('')
      setPassword('')
      setCreateOpen(false)
      setMessage('成员已创建')
      void queryClient.invalidateQueries({ queryKey: ['household-members'] })
    },
    onError: () => setMessage('创建失败，请检查用户名和密码后重试。'),
  })
  const actionMutation = useMutation({
    mutationFn: async () => {
      if (!action) return
      if (action.type === 'deactivate') await deactivateHouseholdMember(action.member.id)
      else await resetHouseholdMemberPassword(action.member.id, replacementPassword)
    },
    onSuccess: () => {
      setMessage(action?.type === 'deactivate' ? '成员已停用' : '密码已重置')
      setAction(null)
      setReplacementPassword('')
      void queryClient.invalidateQueries({ queryKey: ['household-members'] })
    },
    onError: () => setMessage('操作失败，输入已保留。'),
  })

  if (currentUser.isPending || (currentUser.data?.role === 'admin' && members.isPending)) {
    return <div className="household-settings-skeleton skeleton" aria-label="正在加载家庭成员" />
  }
  if (currentUser.isError || members.isError) {
    return <p className="household-settings-error" role="alert">无法读取家庭成员</p>
  }
  if (currentUser.data.role !== 'admin') return null

  return (
    <section className="household-settings" aria-labelledby="household-members-heading">
      <div className="household-settings-heading">
        <div>
          <h2 id="household-members-heading">家庭成员</h2>
          <p>{members.data?.length ?? 0} 名成员</p>
        </div>
        <button type="button" onClick={() => setCreateOpen((value) => !value)} aria-expanded={createOpen}>
          <Plus aria-hidden="true" size={16} />添加成员
        </button>
      </div>

      {createOpen ? (
        <form className="member-create-form" onSubmit={(event) => { event.preventDefault(); createMutation.mutate() }}>
          <label><span>用户名</span><input value={username} onChange={(event) => setUsername(event.target.value)} required /></label>
          <label><span>初始密码</span><input type="password" minLength={12} value={password} onChange={(event) => setPassword(event.target.value)} required /></label>
          <button type="submit" disabled={createMutation.isPending || username.trim() === '' || password.length < 12}>
            {createMutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Check aria-hidden="true" size={16} />}
            创建成员
          </button>
        </form>
      ) : null}

      {message ? <p className="household-settings-message" role="status">{message}</p> : null}
      <ul className="member-list">
        {members.data?.map((member) => (
          <li key={member.id}>
            <span className="member-avatar" aria-hidden="true"><UserRound size={18} /></span>
            <div className="member-identity"><strong>{member.username}</strong><span>{member.role === 'admin' ? '管理员' : '成员'}</span></div>
            <span className={`member-state ${member.active ? 'active' : 'inactive'}`}>
              {member.active ? <Check aria-hidden="true" size={15} /> : <UserX aria-hidden="true" size={15} />}
              {member.active ? '已启用' : '已停用'}
            </span>
            {member.role === 'member' && member.active ? (
              <div className="member-actions">
                <button type="button" aria-label={`重置 ${member.username} 的密码`} onClick={() => setAction({ type: 'reset', member })}><KeyRound aria-hidden="true" size={16} /></button>
                <button type="button" aria-label={`停用 ${member.username}`} onClick={() => setAction({ type: 'deactivate', member })}><UserX aria-hidden="true" size={16} /></button>
              </div>
            ) : <span className="member-role-icon" title="管理员"><Shield aria-hidden="true" size={17} /></span>}
          </li>
        ))}
      </ul>

      <Dialog.Root open={action !== null} onOpenChange={(open) => { if (!open) setAction(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-backdrop" />
          <Dialog.Content
            className="member-dialog"
            onOpenAutoFocus={(event) => { event.preventDefault(); confirmButton.current?.focus() }}
          >
            <Dialog.Title>{action?.type === 'deactivate' ? '停用家庭成员' : '重置成员密码'}</Dialog.Title>
            <Dialog.Description>
              {action?.type === 'deactivate'
                ? `停用 ${action.member.username} 后，其现有会话会立即撤销。`
                : `为 ${action?.member.username ?? ''} 设置新的初始密码。`}
            </Dialog.Description>
            {action?.type === 'reset' ? (
              <label className="dialog-field"><span>新密码</span><input type="password" minLength={12} value={replacementPassword} onChange={(event) => setReplacementPassword(event.target.value)} /></label>
            ) : null}
            <div className="dialog-actions">
              <Dialog.Close asChild><button type="button"><X aria-hidden="true" size={16} />取消</button></Dialog.Close>
              <button
                ref={confirmButton}
                type="button"
                disabled={actionMutation.isPending || (action?.type === 'reset' && replacementPassword.length < 12)}
                onClick={() => actionMutation.mutate()}
              >
                {actionMutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : action?.type === 'deactivate' ? <UserX aria-hidden="true" size={16} /> : <KeyRound aria-hidden="true" size={16} />}
                {action?.type === 'deactivate' ? '确认停用' : '确认重置'}
              </button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </section>
  )
}
