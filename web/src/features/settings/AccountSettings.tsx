import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, LoaderCircle, LogOut, ShieldCheck, UserRound } from 'lucide-react'
import { type FormEvent, useState } from 'react'

import { APIError, changePassword, getCurrentUser, logoutUser } from '../../api/client'

export function AccountSettings() {
  const queryClient = useQueryClient()
  const currentUser = useQuery({ queryKey: ['current-user'], queryFn: ({ signal }) => getCurrentUser(signal) })
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [formError, setFormError] = useState('')
  const [saved, setSaved] = useState(false)
  const logout = useMutation({
    mutationFn: logoutUser,
    onSuccess: async () => {
      sessionStorage.removeItem('video-record.csrf-token')
      await queryClient.resetQueries({ queryKey: ['current-user'], exact: true })
    },
  })
  const passwordMutation = useMutation({
    mutationFn: () => changePassword(currentPassword, newPassword),
    onSuccess: () => {
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setFormError('')
      setSaved(true)
    },
    onError: (error) => {
      setSaved(false)
      if (error instanceof APIError && error.code === 'invalid_credentials') {
        setFormError('当前密码不正确')
        return
      }
      if (error instanceof APIError && error.code === 'invalid_input') {
        setFormError('新密码至少需要 12 个字符')
        return
      }
      setFormError('修改密码失败，请稍后重试')
    },
  })

  if (currentUser.isPending) return <div className="skeleton account-settings-skeleton" aria-label="正在加载当前账户" />
  if (currentUser.isError) return <p className="account-settings-error" role="alert">无法读取当前账户</p>

  const submitPassword = (event: FormEvent) => {
    event.preventDefault()
    setSaved(false)
    if (newPassword !== confirmPassword) {
      setFormError('两次输入的新密码不一致')
      return
    }
    if (newPassword.length < 12) {
      setFormError('新密码至少需要 12 个字符')
      return
    }
    setFormError('')
    passwordMutation.mutate()
  }

  return (
    <section className="account-settings" aria-labelledby="account-settings-heading">
      <div>
        <span className="account-settings-icon"><UserRound aria-hidden="true" size={18} /></span>
        <div>
          <h2 id="account-settings-heading">{currentUser.data.username}</h2>
          <span><ShieldCheck aria-hidden="true" size={14} />{currentUser.data.role === 'admin' ? '管理员' : '家庭成员'}</span>
        </div>
      </div>
      <form className="account-password-form" onSubmit={submitPassword}>
        <h3>修改密码</h3>
        <label className="form-field">
          <span>当前密码</span>
          <input
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
            required
          />
        </label>
        <label className="form-field">
          <span>新密码</span>
          <input
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            minLength={12}
            required
          />
        </label>
        <label className="form-field">
          <span>确认新密码</span>
          <input
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            minLength={12}
            required
          />
        </label>
        {formError ? <p className="account-settings-error" role="alert">{formError}</p> : null}
        {saved ? <p role="status">密码已更新</p> : null}
        <button type="submit" disabled={passwordMutation.isPending}>
          {passwordMutation.isPending
            ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
            : <Check aria-hidden="true" size={16} />}
          {passwordMutation.isPending ? '正在保存' : '保存新密码'}
        </button>
      </form>
      <button type="button" disabled={logout.isPending} onClick={() => logout.mutate()}>
        {logout.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <LogOut aria-hidden="true" size={16} />}
        {logout.isPending ? '正在退出' : '退出登录'}
      </button>
      {logout.isError ? <p className="account-settings-error" role="alert">退出失败，请稍后重试。</p> : null}
    </section>
  )
}
