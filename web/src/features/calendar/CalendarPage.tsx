import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, CalendarDays, RefreshCw } from 'lucide-react'
import { useState } from 'react'

import { getCalendar } from '../../api/client'
import type { CalendarFilter } from '../../api/types'
import { AgendaList } from './AgendaList'
import { MonthGrid } from './MonthGrid'

type CalendarPageProps = {
  now?: Date
  timezone?: string
}

const filters = [
  { value: 'all', label: '全部记录' },
  { value: 'completed', label: '只看看完' },
  { value: 'in_progress', label: '只看进行中' },
] satisfies Array<{ value: CalendarFilter; label: string }>

export function CalendarPage({
  now = new Date(),
  timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
}: CalendarPageProps) {
  const [month, setMonth] = useState(() => monthInTimezone(now, timezone))
  const [filter, setFilter] = useState<CalendarFilter>('all')
  const calendar = useQuery({
    queryKey: ['calendar', month, timezone, filter],
    queryFn: ({ signal }) => getCalendar(month, timezone, filter, signal),
  })
  const [year, monthNumber] = month.split('-').map(Number)
  const moveMonth = (offset: number) => setMonth(shiftMonth(month, offset))

  return (
    <div className="page calendar-page">
      <header className="calendar-page-header">
        <div>
          <p className="page-kicker">观看时间线</p>
          <h1>{year}年{monthNumber}月</h1>
        </div>
        <div className="calendar-month-actions" aria-label="切换月份">
          <button type="button" aria-label="上个月" onClick={() => moveMonth(-1)}><ArrowLeft aria-hidden="true" size={18} /></button>
          <button type="button" onClick={() => setMonth(monthInTimezone(now, timezone))}>今天</button>
          <button type="button" aria-label="下个月" onClick={() => moveMonth(1)}><ArrowRight aria-hidden="true" size={18} /></button>
        </div>
      </header>

      <div className="calendar-filters" role="group" aria-label="日历筛选">
        {filters.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={filter === item.value}
            onClick={() => setFilter(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>

      {calendar.isPending ? <CalendarSkeleton /> : null}
      {calendar.isError ? (
        <div className="calendar-error" role="alert">
          <CalendarDays aria-hidden="true" size={22} />
          <p>无法读取日历，已有记录仍安全保存在服务器中。</p>
          <button type="button" disabled={calendar.isFetching} onClick={() => void calendar.refetch()}>
            <RefreshCw className={calendar.isFetching ? 'loading-icon' : ''} aria-hidden="true" size={16} />
            重试日历
          </button>
        </div>
      ) : null}
      {calendar.data ? (
        calendar.data.events.length > 0 ? (
          <>
            <MonthGrid year={calendar.data.year} month={calendar.data.month} events={calendar.data.events} />
            <AgendaList events={calendar.data.events} timezone={calendar.data.timezone} />
          </>
        ) : (
          <div className="empty-state calendar-empty">
            <CalendarDays aria-hidden="true" size={24} />
            <p>这个月还没有符合筛选条件的记录</p>
          </div>
        )
      ) : null}
    </div>
  )
}

function CalendarSkeleton() {
  return (
    <div className="calendar-skeleton" aria-label="正在加载日历">
      <div className="skeleton calendar-skeleton-header" />
      <div className="skeleton calendar-skeleton-body" />
    </div>
  )
}

function monthInTimezone(now: Date, timezone: string) {
  const parts = new Intl.DateTimeFormat('en', {
    year: 'numeric', month: '2-digit', timeZone: timezone,
  }).formatToParts(now)
  const year = parts.find((part) => part.type === 'year')?.value ?? String(now.getUTCFullYear())
  const month = parts.find((part) => part.type === 'month')?.value ?? String(now.getUTCMonth() + 1).padStart(2, '0')
  return `${year}-${month}`
}

function shiftMonth(value: string, offset: number) {
  const [yearText, monthText] = value.split('-')
  const year = Number(yearText ?? 0)
  const month = Number(monthText ?? 1)
  const shifted = new Date(Date.UTC(year, month - 1 + offset, 1))
  return `${shifted.getUTCFullYear()}-${String(shifted.getUTCMonth() + 1).padStart(2, '0')}`
}
