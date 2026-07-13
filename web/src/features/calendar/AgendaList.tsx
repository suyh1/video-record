import { CalendarDays, Users } from 'lucide-react'
import { NavLink } from 'react-router-dom'

import type { CalendarEvent } from '../../api/types'

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
