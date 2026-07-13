import { NavLink } from 'react-router-dom'

import type { CalendarEvent } from '../../api/types'

type MonthGridProps = {
  year: number
  month: number
  events: CalendarEvent[]
}

const weekDays = ['一', '二', '三', '四', '五', '六', '日']

export function MonthGrid({ year, month, events }: MonthGridProps) {
  const eventsByDay = new Map<number, CalendarEvent[]>()
  for (const event of events) {
    const day = Number(event.localDate.slice(8, 10))
    eventsByDay.set(day, [...(eventsByDay.get(day) ?? []), event])
  }
  const cells = monthCells(year, month)
  return (
    <table className="calendar-month-grid" aria-label={`${year}年${month}月观影日历`}>
      <thead>
        <tr>{weekDays.map((day) => <th key={day} scope="col">周{day}</th>)}</tr>
      </thead>
      <tbody>
        {Array.from({ length: 6 }, (_, row) => (
          <tr key={row}>
            {cells.slice(row * 7, row * 7 + 7).map((day, column) => {
              const dayEvents = day === null ? [] : eventsByDay.get(day) ?? []
              return (
                <td
                  key={`${row}-${column}`}
                  className={day === null ? 'outside-month' : ''}
                  aria-label={day === null ? undefined : `${month}月${day}日，${dayEvents.length} 条记录`}
                >
                  {day === null ? null : (
                    <>
                      <span className="calendar-day-number">{day}</span>
                      <ol>
                        {dayEvents.map((event) => (
                          <li key={event.id}>
                            <NavLink to={`/media/${event.mediaId}`} title={eventTitle(event)}>
                              <span>{event.title}</span>
                              {event.seasonNumber !== null && event.episodeNumber !== null ? (
                                <small>{episodeCode(event)}</small>
                              ) : null}
                            </NavLink>
                          </li>
                        ))}
                      </ol>
                    </>
                  )}
                </td>
              )
            })}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function monthCells(year: number, month: number) {
  const firstWeekDay = new Date(Date.UTC(year, month - 1, 1)).getUTCDay()
  const leading = (firstWeekDay + 6) % 7
  const days = new Date(Date.UTC(year, month, 0)).getUTCDate()
  return Array.from({ length: 42 }, (_, index) => {
    const day = index - leading + 1
    return day >= 1 && day <= days ? day : null
  })
}

function eventTitle(event: CalendarEvent) {
  return event.seasonNumber !== null && event.episodeNumber !== null
    ? `${event.title} ${episodeCode(event)}`
    : event.title
}

function episodeCode(event: CalendarEvent) {
  return `S${String(event.seasonNumber).padStart(2, '0')}E${String(event.episodeNumber).padStart(2, '0')}`
}
