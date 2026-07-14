import { useQueries } from '@tanstack/react-query'
import { MessageSquareText, Star } from 'lucide-react'

import { getVisibleHouseholdRecord } from '../../api/client'
import type { HouseholdMember } from '../../api/types'

type HouseholdSharedRecordsProps = {
  mediaID: string
  members: HouseholdMember[]
}

export function HouseholdSharedRecords({ mediaID, members }: HouseholdSharedRecordsProps) {
  const queries = useQueries({
    queries: members.map((member) => ({
      queryKey: ['household-visible-record', member.id, mediaID],
      queryFn: ({ signal }: { signal: AbortSignal }) => getVisibleHouseholdRecord(member.id, mediaID, signal),
    })),
  })
  if (members.length > 0 && queries.some((query) => query.isPending)) {
    return <div className="household-shared-skeleton skeleton" aria-label="正在加载家庭评价" />
  }
  const visible = queries.flatMap((query, index) => {
    const record = query.data
    const member = members[index]
    return record && member && (record.rating !== null || record.sharedReview)
      ? [{ member, record }]
      : []
  })
  if (!visible.length) return null

  return (
    <section className="details-section household-shared-records" aria-labelledby="household-shared-heading">
      <div className="details-section-heading">
        <div><h2 id="household-shared-heading">家庭评价</h2><p>仅显示成员主动公开的内容</p></div>
      </div>
      <ul>
        {visible.map(({ member, record }) => (
          <li key={member.id}>
            <strong>{member.username}</strong>
            {record.rating !== null ? <span><Star aria-hidden="true" size={15} />{record.rating.toFixed(1)} / 10</span> : null}
            {record.sharedReview ? <p><MessageSquareText aria-hidden="true" size={16} />{record.sharedReview}</p> : null}
          </li>
        ))}
      </ul>
    </section>
  )
}
