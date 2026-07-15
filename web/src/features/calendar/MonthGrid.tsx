import { NavLink } from 'react-router-dom'

import type { CalendarEvent } from '../../api/types'

type MonthGridProps = {
  active: boolean
  year: number
  month: number
  events: CalendarEvent[]
  todayDate: string
  selectedDate: string | null
  onSelectDate: (date: string) => void
}

const weekDays = ['一', '二', '三', '四', '五', '六', '日']

export function MonthGrid({ active, year, month, events, todayDate, selectedDate, onSelectDate }: MonthGridProps) {
  const eventsByDay = new Map<number, CalendarEvent[]>()
  for (const event of events) {
    const day = Number(event.localDate.slice(8, 10))
    eventsByDay.set(day, [...(eventsByDay.get(day) ?? []), event])
  }
  const cells = monthCells(year, month)
  return (
    <table
      id="calendar-month-view"
      className={`calendar-month-grid${active ? ' is-active' : ''}`}
      aria-label={`${year}年${month}月观影日历`}
    >
      <thead>
        <tr>{weekDays.map((day) => <th key={day} scope="col">周{day}</th>)}</tr>
      </thead>
      <tbody>
        {Array.from({ length: 6 }, (_, row) => (
          <tr key={row}>
            {cells.slice(row * 7, row * 7 + 7).map((day, column) => {
              const dayEvents = day === null ? [] : eventsByDay.get(day) ?? []
              const localDate = day === null ? null : calendarDate(year, month, day)
              const today = localDate === todayDate
              const selected = localDate === selectedDate
              const hasEvents = dayEvents.length > 0
              return (
                <td
                  key={`${row}-${column}`}
                  className={day === null
                    ? 'outside-month'
                    : [hasEvents ? 'has-events' : '', today ? 'is-today' : '', selected ? 'is-selected' : ''].filter(Boolean).join(' ')}
                  aria-label={day === null ? undefined : `${month}月${day}日，${dayEvents.length} 条记录`}
                >
                  {day === null || localDate === null ? null : (
                    <>
                      <button
                        type="button"
                        className="calendar-day-button"
                        aria-current={today ? 'date' : undefined}
                        aria-label={`${month}月${day}日，${dayEvents.length} 条记录`}
                        aria-pressed={selected}
                        data-has-events={hasEvents}
                        onClick={() => onSelectDate(localDate)}
                      >
                        <span className="calendar-day-number">{day}</span>
                        {hasEvents ? <span className="calendar-day-count">{dayEvents.length} 条</span> : null}
                      </button>
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

function calendarDate(year: number, month: number, day: number) {
  return `${year}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`
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
