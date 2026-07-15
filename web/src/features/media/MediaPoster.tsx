import { Bookmark, Check, CircleStop, Play } from 'lucide-react'
import { useState } from 'react'

import type { MediaSearchResult, RecordStatus } from '../../api/types'
import { mediaImageURL } from '../../lib/mediaImage'

const statusLabels: Record<RecordStatus, string> = {
  none: '未记录',
  wishlist: '想看',
  watching: '在看',
  completed: '看过',
  dropped: '弃看',
}

const statusIcons = {
  wishlist: Bookmark,
  watching: Play,
  completed: Check,
  dropped: CircleStop,
}

export function MediaPoster({ item, compact = false }: { item: MediaSearchResult; compact?: boolean }) {
  const StatusIcon = item.status === 'none' ? null : statusIcons[item.status]
  const posterURL = mediaImageURL(item.posterPath)

  return (
    <div className={`media-poster${compact ? ' compact' : ''}`}>
      <div className="poster-frame">
        <PosterArtwork key={`${item.id}:${item.title}:${posterURL ?? ''}`} title={item.title} posterURL={posterURL} />
      </div>
      <div className="poster-copy">
        <strong>{item.title}</strong>
        <span className="poster-original-title">{item.originalTitle}</span>
        <span className="poster-metadata">
          <span>{item.mediaType === 'movie' ? '电影' : '剧集'}</span>
          {item.year ? <span>{item.year}</span> : null}
          {item.status !== 'none' ? (
            <span className={`record-status ${item.status}`}>
              {StatusIcon ? <StatusIcon aria-hidden="true" size={14} /> : null}
              {statusLabels[item.status]}
            </span>
          ) : null}
        </span>
      </div>
    </div>
  )
}

function PosterArtwork({ title, posterURL }: { title: string; posterURL: string | null }) {
  const [failed, setFailed] = useState(false)

  if (!posterURL || failed) {
    return (
      <span className="poster-placeholder" aria-label={`${title} 暂无海报`}>
        {Array.from(title)[0] ?? '影'}
      </span>
    )
  }

  return <img src={posterURL} alt={`${title} 海报`} loading="lazy" onError={() => setFailed(true)} />
}
