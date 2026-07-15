import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, CalendarDays, RefreshCw } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import { getCalendar } from '../../api/client'
import type { CalendarFilter } from '../../api/types'
import { BrandMark } from '../../app/BrandMark'
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
  const [view, setView] = useState<'agenda' | 'month'>('agenda')
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  const [agendaFocusRequest, setAgendaFocusRequest] = useState(0)
  const agendaViewRef = useRef<HTMLDivElement>(null)
  const calendar = useQuery({
    queryKey: ['calendar', month, timezone, filter],
    queryFn: ({ signal }) => getCalendar(month, timezone, filter, signal),
  })
  const [year, monthNumber] = month.split('-').map(Number)
  const todayDate = dateInTimezone(now, timezone)
  const displayMonth = (value: string) => {
    setSelectedDate(null)
    setMonth(value)
  }
  const moveMonth = (offset: number) => displayMonth(shiftMonth(month, offset))
  const requestAgendaFocus = () => setAgendaFocusRequest((request) => request + 1)

  useEffect(() => {
    if (agendaFocusRequest > 0) agendaViewRef.current?.focus()
  }, [agendaFocusRequest])

  return (
    <div className="page calendar-page">
      <header className="calendar-page-header">
        <div>
          <p className="page-kicker">观看时间线</p>
          <h1>{year}年{monthNumber}月</h1>
        </div>
        <div className="calendar-month-actions" aria-label="切换月份">
          <button type="button" aria-label="上个月" onClick={() => moveMonth(-1)}><ArrowLeft aria-hidden="true" size={18} /></button>
          <button type="button" onClick={() => displayMonth(monthInTimezone(now, timezone))}>今天</button>
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

      {calendar.data?.events.length ? (
        <div className="calendar-view-switch" role="group" aria-label="日历视图">
          <button
            type="button"
            aria-controls="calendar-agenda-view"
            aria-pressed={view === 'agenda'}
            onClick={() => setView('agenda')}
          >
            日程
          </button>
          <button
            type="button"
            aria-controls="calendar-month-view"
            aria-pressed={view === 'month'}
            onClick={() => setView('month')}
          >
            月历
          </button>
        </div>
      ) : null}

      {selectedDate ? (
        <div className="calendar-selection-summary">
          <p>{formatCalendarDate(selectedDate)}日程</p>
          <button
            type="button"
            onClick={() => {
              setSelectedDate(null)
              setView('agenda')
              requestAgendaFocus()
            }}
          >
            查看全月
          </button>
        </div>
      ) : null}

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
          <div className="calendar-views">
            <MonthGrid
              active={view === 'month'}
              events={calendar.data.events}
              month={calendar.data.month}
              onSelectDate={(date) => {
                setSelectedDate(date)
                setView('agenda')
                requestAgendaFocus()
              }}
              selectedDate={selectedDate}
              todayDate={todayDate}
              year={calendar.data.year}
            />
            <div
              ref={agendaViewRef}
              id="calendar-agenda-view"
              className={`calendar-agenda-view${view === 'agenda' ? ' is-active' : ''}`}
              role="region"
              aria-label="日程视图"
              tabIndex={-1}
            >
              {selectedDate && !calendar.data.events.some((event) => event.localDate === selectedDate) ? (
                <div className="calendar-agenda-empty" role="status">
                  {formatCalendarDate(selectedDate)}暂无观看记录
                </div>
              ) : (
                <AgendaList
                  events={selectedDate
                    ? calendar.data.events.filter((event) => event.localDate === selectedDate)
                    : calendar.data.events}
                  timezone={calendar.data.timezone}
                />
              )}
            </div>
          </div>
        ) : (
          <div className="empty-state page-empty-state calendar-empty" role="region" aria-label="日历暂无记录">
            <BrandMark size={28} />
            <p>这个月还没有符合条件的观看记录。</p>
            <Link className="text-link" to="/library">去影库记录</Link>
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
  return dateInTimezone(now, timezone).slice(0, 7)
}

function dateInTimezone(now: Date, timezone: string) {
  const parts = new Intl.DateTimeFormat('en', {
    year: 'numeric', month: '2-digit', day: '2-digit', timeZone: timezone,
  }).formatToParts(now)
  const year = parts.find((part) => part.type === 'year')?.value ?? String(now.getUTCFullYear())
  const month = parts.find((part) => part.type === 'month')?.value ?? String(now.getUTCMonth() + 1).padStart(2, '0')
  const day = parts.find((part) => part.type === 'day')?.value ?? String(now.getUTCDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function shiftMonth(value: string, offset: number) {
  const [yearText, monthText] = value.split('-')
  const year = Number(yearText ?? 0)
  const month = Number(monthText ?? 1)
  const shifted = new Date(Date.UTC(year, month - 1 + offset, 1))
  return `${shifted.getUTCFullYear()}-${String(shifted.getUTCMonth() + 1).padStart(2, '0')}`
}

function formatCalendarDate(value: string) {
  const [, month, day] = value.split('-').map(Number)
  return `${month}月${day}日`
}
