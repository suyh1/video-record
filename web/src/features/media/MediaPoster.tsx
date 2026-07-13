import { Bookmark, Check, CircleStop, Play } from 'lucide-react'

import type { MediaSearchResult, RecordStatus } from '../../api/types'

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
  const posterURL = imageURL(item.posterPath)

  return (
    <div className={`media-poster${compact ? ' compact' : ''}`}>
      <div className="poster-frame">
        {posterURL ? (
          <img src={posterURL} alt={`${item.title} 海报`} loading="lazy" />
        ) : (
          <span className="poster-placeholder" aria-label={`${item.title} 暂无海报`}>
            {Array.from(item.title)[0] ?? '影'}
          </span>
        )}
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

function imageURL(path: string | null) {
  if (!path) return null
  if (/^https?:\/\//.test(path)) return path
  return `https://image.tmdb.org/t/p/w342${path}`
}
