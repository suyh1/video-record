import { useQuery } from '@tanstack/react-query'
import { Ban, CheckCircle2, CircleAlert, Clock3, RefreshCw, Server } from 'lucide-react'
import { Link } from 'react-router-dom'

import { getSyncStatus } from '../../api/client'
import type { SyncAccountStatus } from '../../api/types'

export function SyncStatus() {
  const status = useQuery({
    queryKey: ['sync-status'],
    queryFn: ({ signal }) => getSyncStatus(signal),
  })

  if (status.isPending) {
    return <div className="sync-status-skeleton skeleton" aria-label="正在加载媒体服务器同步状态" />
  }

  return (
    <section className="sync-status" aria-labelledby="sync-status-heading">
      <div className="sync-status-heading">
        <div>
          <h2 id="sync-status-heading">媒体服务器同步</h2>
          <p>播放历史在后台同步，存在冲突时由你决定如何落档。</p>
        </div>
        {status.data && status.data.pendingTotal > 0 ? (
          <Link to="/settings/sync">核对 {status.data.pendingTotal} 条候选</Link>
        ) : null}
      </div>

      {status.isError ? (
        <div className="sync-status-error" role="alert">
          <span><CircleAlert aria-hidden="true" size={18} />无法读取同步状态</span>
          <button type="button" onClick={() => void status.refetch()}>
            <RefreshCw aria-hidden="true" size={16} />重试
          </button>
        </div>
      ) : status.data.accounts.length === 0 ? (
        <p className="sync-status-empty">还没有媒体服务器集成</p>
      ) : (
        <ul className="sync-account-list" aria-label="媒体服务器账户">
          {status.data.accounts.map((account) => (
            <li key={account.id}>
              <Server aria-hidden="true" size={18} />
              <div>
                <strong>{providerName(account.provider)} · {account.name}</strong>
                <span>{account.pendingCandidates > 0 ? `${account.pendingCandidates} 条待核对` : '没有待核对记录'}</span>
              </div>
              <AccountRunStatus account={account} />
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function AccountRunStatus({ account }: { account: SyncAccountStatus }) {
  if (!account.enabled) {
    return <span className="sync-run-state muted"><Ban aria-hidden="true" size={16} />已停用</span>
  }
  if (account.lastRunStatus === 'succeeded') {
    return <span className="sync-run-state success"><CheckCircle2 aria-hidden="true" size={16} />同步成功</span>
  }
  if (account.lastRunStatus === 'failed') {
    return <span className="sync-run-state error"><CircleAlert aria-hidden="true" size={16} />同步失败</span>
  }
  if (account.lastRunStatus === 'running') {
    return <span className="sync-run-state info"><RefreshCw aria-hidden="true" size={16} />正在同步</span>
  }
  return <span className="sync-run-state muted"><Clock3 aria-hidden="true" size={16} />等待首次同步</span>
}

function providerName(provider: SyncAccountStatus['provider']) {
  if (provider === 'jellyfin') return 'Jellyfin'
  if (provider === 'emby') return 'Emby'
  return 'Plex'
}
