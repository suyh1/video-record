import { Link } from 'react-router-dom'

import type { StatsPoint } from '../../api/types'

type AccessibleChartProps = {
  title: string
  chartLabel: string
  tableLabel: string
  points: StatsPoint[]
  valueSuffix?: string
  drillParam?: 'tag' | 'genre' | 'method'
}

export function AccessibleChart({
  title,
  chartLabel,
  tableLabel,
  points,
  valueSuffix = '次',
  drillParam,
}: AccessibleChartProps) {
  const maximum = Math.max(...points.map((point) => point.value), 1)
  return (
    <section className="stats-section" aria-labelledby={`stats-${tableLabel}`}>
      <h2 id={`stats-${tableLabel}`}>{title}</h2>
      <div className="accessible-chart-layout">
        <div className="accessible-bars" role="img" aria-label={chartLabel}>
          {points.length > 0 ? points.map((point) => (
            <div className="accessible-bar-row" key={point.label}>
              <span>
                {drillParam === 'tag' ? (
                  <Link to={`/library?tag=${encodeURIComponent(point.label)}`}>{point.label}</Link>
                ) : point.label}
              </span>
              <div aria-hidden="true"><i style={{ width: `${Math.max(2, (point.value / maximum) * 100)}%` }} /></div>
              <strong>{point.value} {valueSuffix}</strong>
            </div>
          )) : <p>暂无数据</p>}
        </div>
        <table className="stats-data-table" aria-label={tableLabel}>
          <thead><tr><th scope="col">项目</th><th scope="col">数值</th></tr></thead>
          <tbody>
            {points.length > 0 ? points.map((point) => (
              <tr key={point.label}><th scope="row">{point.label}</th><td>{point.value} {valueSuffix}</td></tr>
            )) : <tr><td colSpan={2}>暂无数据</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  )
}
