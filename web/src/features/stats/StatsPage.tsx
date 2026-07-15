import { useQuery } from '@tanstack/react-query'
import { BarChart3, RefreshCw } from 'lucide-react'
import { Link } from 'react-router-dom'

import { getStats } from '../../api/client'
import { BrandMark } from '../../app/BrandMark'
import { AccessibleChart } from './AccessibleChart'

type StatsPageProps = {
  timezone?: string
}

export function StatsPage({
  timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
}: StatsPageProps) {
  const stats = useQuery({
    queryKey: ['stats', timezone],
    queryFn: ({ signal }) => getStats(timezone, signal),
  })

  if (stats.isPending) return <StatsSkeleton />
  if (stats.isError) {
    return (
      <div className="page stats-page">
        <header className="page-heading"><p className="page-kicker">私人回顾</p><h1>统计</h1></header>
        <div className="stats-error" role="alert">
          <BarChart3 aria-hidden="true" size={24} />
          <p>无法读取统计数据，请稍后重试。</p>
          <button type="button" disabled={stats.isFetching} onClick={() => void stats.refetch()}>
            <RefreshCw className={stats.isFetching ? 'loading-icon' : ''} aria-hidden="true" size={16} />
            重试统计
          </button>
        </div>
      </div>
    )
  }

  const summary = stats.data
  if (summary.totalWatches === 0) {
    return (
      <div className="page stats-page">
        <header className="page-heading stats-page-heading">
          <p className="page-kicker">私人回顾</p>
          <h1>统计</h1>
        </header>
        <div className="empty-state page-empty-state stats-empty" role="region" aria-label="统计暂无记录">
          <BrandMark size={28} />
          <p>记录第一次观看后，这里会显示你的观影视图。</p>
          <Link className="text-link" to="/library">去影库记录</Link>
        </div>
      </div>
    )
  }
  return (
    <div className="page stats-page">
      <header className="page-heading stats-page-heading">
        <p className="page-kicker">私人回顾</p>
        <h1>统计</h1>
      </header>
      <dl className="stats-overview" aria-label="观影概览">
        <div><dt>观看</dt><dd>{summary.totalWatches} 次观看</dd></div>
        <div><dt>作品</dt><dd>{summary.uniqueMedia} 部作品</dd></div>
        <div><dt>时长</dt><dd>{formatMinutes(summary.totalMinutes)}</dd></div>
        <div><dt>重复观看</dt><dd>{summary.repeatWatches} 次重看</dd></div>
      </dl>

      <AccessibleChart title="月度观看" chartLabel="月度观看图" tableLabel="月度观看数据" points={summary.monthly} />
      <AccessibleChart title="年度观看" chartLabel="年度观看图" tableLabel="年度观看数据" points={summary.yearly} />
      <AccessibleChart title="类型分布" chartLabel="类型分布图" tableLabel="类型分布数据" points={summary.genres} />
      <AccessibleChart title="评分分布" chartLabel="评分分布图" tableLabel="评分分布数据" points={summary.ratings} valueSuffix="部" />
      <AccessibleChart title="标签分布" chartLabel="标签分布图" tableLabel="标签分布数据" points={summary.tags} valueSuffix="部" />
      <AccessibleChart title="观看方式" chartLabel="观看方式图" tableLabel="观看方式数据" points={summary.viewingMethods} />
    </div>
  )
}

function StatsSkeleton() {
  return (
    <div className="page stats-page" aria-label="正在加载统计">
      <div className="skeleton stats-heading-skeleton" />
      <div className="skeleton stats-overview-skeleton" />
      <div className="skeleton stats-chart-skeleton" />
    </div>
  )
}

function formatMinutes(totalMinutes: number) {
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return `${hours} 小时 ${minutes} 分`
}
