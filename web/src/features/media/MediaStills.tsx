import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import { getTMDBImages } from '../../api/client'
import type { MediaType } from '../../api/types'
import { mediaImageURL } from '../../lib/mediaImage'

type MediaStillsProps = {
  mediaType: MediaType
  tmdbId: number | null
}

export function MediaStills({ mediaType, tmdbId }: MediaStillsProps) {
  const [open, setOpen] = useState(false)
  const images = useQuery({
    queryKey: ['tmdb-images', mediaType, tmdbId],
    queryFn: ({ signal }) => getTMDBImages(mediaType, tmdbId ?? 0, signal),
    enabled: open && tmdbId !== null,
  })
  if (tmdbId === null) return null

  return (
    <details className="media-stills" open={open} onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary>剧照</summary>
      {!open ? null : images.isPending ? (
        <div className="media-stills-strip" aria-label="正在加载剧照">
          {Array.from({ length: 4 }, (_, index) => <div key={index} className="skeleton media-still-skeleton" />)}
        </div>
      ) : images.isError ? (
        <p className="quiet-empty" role="alert">剧照暂时不可用</p>
      ) : (images.data?.backdrops.length ?? 0) === 0 ? (
        <p className="quiet-empty">TMDB 暂无剧照</p>
      ) : (
        <ul className="media-stills-strip" aria-label="剧照列表">
          {images.data!.backdrops.map((item) => {
            const url = mediaImageURL(item.url) ?? item.url
            return (
              <li key={item.url}>
                <img src={url} alt="" loading="lazy" />
              </li>
            )
          })}
        </ul>
      )}
    </details>
  )
}
