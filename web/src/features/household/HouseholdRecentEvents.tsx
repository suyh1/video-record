import { useQuery } from '@tanstack/react-query'
import { Users } from 'lucide-react'
import { Link } from 'react-router-dom'

import { getHouseholdEvents } from '../../api/client'
import { formatLocalSeconds } from '../../lib/dateTime'

export function HouseholdRecentEvents() {
  const events = useQuery({
    queryKey: ['household-events'],
    queryFn: ({ signal }) => getHouseholdEvents(signal),
  })

  return (
    <section className="household-recent-events" aria-labelledby="household-events-heading">
      <div>
        <h2 id="household-events-heading">家庭共同观看</h2>
        <p>只读回看家庭成员一起看过的作品，不含私人笔记。</p>
      </div>
      {events.isPending ? <div className="skeleton" aria-label="正在加载家庭共同观看" /> : null}
      {events.isError ? <p role="alert">无法读取家庭共同观看记录</p> : null}
      {events.data && events.data.length === 0 ? <p className="quiet-empty">还没有共同观看记录</p> : null}
      {events.data && events.data.length > 0 ? (
        <ul>
          {events.data.slice(0, 12).map((event) => (
            <li key={event.id}>
              <Users aria-hidden="true" size={16} />
              <div>
                <Link to={`/media/${event.mediaId}`}>{event.title}</Link>
                <span>{formatLocalSeconds(event.watchedAt)}</span>
                <span>与 {event.participants.join('、')}</span>
              </div>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )
}
