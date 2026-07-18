import { Bookmark, CalendarDays, Check, CircleStop, Play, Users } from 'lucide-react'
import { NavLink } from 'react-router-dom'

import type { CalendarEvent, RecordStatus } from '../../api/types'

type AgendaListProps = {
  events: CalendarEvent[]
  timezone: string
}

export function AgendaList({ events, timezone }: AgendaListProps) {
  const groups = groupEvents(events)
  return (
    <ol className="calendar-agenda" aria-label="按日议程">
      {groups.map(({ date, events: dayEvents }) => (
        <li key={date} aria-label={formatDay(date)}>
          <div className="agenda-day-heading">
            <CalendarDays aria-hidden="true" size={18} />
            <h2>{formatDay(date)}</h2>
            <span>{dayEvents.length} 条</span>
          </div>
          <ol className="agenda-events">
            {dayEvents.map((event) => (
              <li key={event.id}>
                <time dateTime={event.watchedAt}>{formatTime(event.watchedAt, timezone)}</time>
                <div>
                  <NavLink to={`/media/${event.mediaId}`}>{event.title}</NavLink>
                  <span className={`record-status ${event.status}`}>
                    <StatusIcon status={event.status} />
                    {statusLabel(event.status)}
                  </span>
                  {event.seasonNumber !== null && event.episodeNumber !== null && event.absoluteNumber !== null ? (
                    <span>{episodeLabel(event)}</span>
                  ) : null}
                  {event.viewingMethod ? <span>{event.viewingMethod}</span> : null}
                  {event.participants.length > 1 ? (
                    <span className="agenda-participants"><Users aria-hidden="true" size={14} />与 {event.participants.join('、')}共同观看</span>
                  ) : null}
                </div>
              </li>
            ))}
          </ol>
        </li>
      ))}
    </ol>
  )
}

function groupEvents(events: CalendarEvent[]) {
  const grouped = new Map<string, CalendarEvent[]>()
  for (const event of events) grouped.set(event.localDate, [...(grouped.get(event.localDate) ?? []), event])
  return [...grouped.entries()]
    .sort(([left], [right]) => right.localeCompare(left))
    .map(([date, dayEvents]) => ({
      date,
      events: [...dayEvents].sort((left, right) => right.watchedAt.localeCompare(left.watchedAt)),
    }))
}

function formatDay(value: string) {
  const [, month, day] = value.split('-').map(Number)
  return `${month}月${day}日`
}

function formatTime(value: string, timezone: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit', minute: '2-digit', hour12: false, timeZone: timezone,
  }).format(new Date(value))
}

function episodeLabel(event: CalendarEvent) {
  const code = `S${String(event.seasonNumber).padStart(2, '0')}E${String(event.episodeNumber).padStart(2, '0')}`
  return `${code} · 全剧第 ${event.absoluteNumber} 集`
}

const statusLabels: Record<RecordStatus, string> = {
  none: '未记录',
  wishlist: '想看',
  watching: '在看',
  completed: '看过',
  dropped: '弃看',
}

function statusLabel(status: RecordStatus) {
  return statusLabels[status]
}

function StatusIcon({ status }: { status: RecordStatus }) {
  if (status === 'wishlist') return <Bookmark aria-hidden="true" size={14} />
  if (status === 'watching') return <Play aria-hidden="true" size={14} />
  if (status === 'completed') return <Check aria-hidden="true" size={14} />
  if (status === 'dropped') return <CircleStop aria-hidden="true" size={14} />
  return null
}
