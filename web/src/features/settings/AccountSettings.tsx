import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LoaderCircle, LogOut, ShieldCheck, UserRound } from 'lucide-react'

import { getCurrentUser, logoutUser } from '../../api/client'

export function AccountSettings() {
  const queryClient = useQueryClient()
  const currentUser = useQuery({ queryKey: ['current-user'], queryFn: ({ signal }) => getCurrentUser(signal) })
  const logout = useMutation({
    mutationFn: logoutUser,
    onSuccess: async () => {
      sessionStorage.removeItem('video-record.csrf-token')
      await queryClient.resetQueries({ queryKey: ['current-user'], exact: true })
    },
  })

  if (currentUser.isPending) return <div className="skeleton account-settings-skeleton" aria-label="正在加载当前账户" />
  if (currentUser.isError) return <p className="account-settings-error" role="alert">无法读取当前账户</p>

  return (
    <section className="account-settings" aria-labelledby="account-settings-heading">
      <div>
        <span className="account-settings-icon"><UserRound aria-hidden="true" size={18} /></span>
        <div>
          <h2 id="account-settings-heading">{currentUser.data.username}</h2>
          <span><ShieldCheck aria-hidden="true" size={14} />{currentUser.data.role === 'admin' ? '管理员' : '家庭成员'}</span>
        </div>
      </div>
      <button type="button" disabled={logout.isPending} onClick={() => logout.mutate()}>
        {logout.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <LogOut aria-hidden="true" size={16} />}
        {logout.isPending ? '正在退出' : '退出登录'}
      </button>
      {logout.isError ? <p className="account-settings-error" role="alert">退出失败，请稍后重试。</p> : null}
    </section>
  )
}
