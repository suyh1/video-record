import { Star } from 'lucide-react'
import { type ReactNode, useState } from 'react'

import type { MediaDetails, MediaSearchResult, RecordState, TMDBMovieDetails, TMDBTVDetails } from '../../api/types'
import { type MediaPalette, sampleMediaPalette } from '../../lib/mediaAccent'
import { mediaImageURL } from '../../lib/mediaImage'
import { MediaPoster } from './MediaPoster'

type MediaHeroProps = {
  media: MediaDetails
  record: RecordState
  external?: TMDBMovieDetails | TMDBTVDetails | undefined
  linker: ReactNode
  onPaletteChange?: (palette: MediaPalette | null) => void
}

export function MediaHero({ media, record, external, linker, onPaletteChange }: MediaHeroProps) {
  const isMovie = media.mediaType === 'movie'
  const liveTitle = external ? ('title' in external ? external.title : external.name) : ''
  const liveOriginalTitle = external ? ('originalTitle' in external ? external.originalTitle : external.originalName) : ''
  const liveDate = external ? ('releaseDate' in external ? external.releaseDate : external.firstAirDate) : ''
  const runtime = external
    ? ('runtime' in external ? external.runtime : external.episodeRuntime[0] ?? media.runtimeMinutes)
    : media.runtimeMinutes
  const title = media.title || liveTitle
  const originalTitle = liveOriginalTitle || media.originalTitle
  const releaseDate = liveDate || media.releaseDate
  const posterPath = external?.posterPath || media.posterPath
  const backdropPath = external?.backdropPath || media.backdropPath
  const backdropURL = mediaImageURL(backdropPath)
  const backdropIdentity = `${media.id}:${title}:${backdropURL ?? ''}`
  const [failedBackdrop, setFailedBackdrop] = useState<string | null>(null)
  const [loadedBackdrop, setLoadedBackdrop] = useState<string | null>(null)
  const backdropFailed = failedBackdrop === backdropIdentity
  const hasBackdrop = Boolean(backdropURL && !backdropFailed && loadedBackdrop === backdropIdentity)
  const overview = external?.overview || media.overview || '暂无简介'
  const genres = external?.genres.length ? external.genres : media.genres
  const posterItem: MediaSearchResult = {
    id: media.id,
    tmdbId: media.tmdbId,
    source: 'local',
    mediaType: media.mediaType,
    title,
    originalTitle,
    year: releaseDate.slice(0, 4),
    posterPath,
    status: record.status,
  }

  return (
    <header
      className={`media-hero${hasBackdrop ? ' has-backdrop' : ''}`}
      data-backdrop-state={hasBackdrop ? 'ready' : backdropFailed ? 'failed' : backdropURL ? 'loading' : 'empty'}
    >
      {backdropURL && !backdropFailed ? (
        <img
          key={backdropIdentity}
          className="media-hero-backdrop"
          src={backdropURL}
          alt=""
          loading="eager"
          fetchPriority="high"
          onLoad={() => {
            setLoadedBackdrop(backdropIdentity)
          }}
          onError={() => {
            setFailedBackdrop(backdropIdentity)
          }}
        />
      ) : null}
      <div className="media-hero-shade" aria-hidden="true" />
      <div className="media-hero-content">
        <MediaPoster
          item={posterItem}
          onArtworkLoad={(image) => onPaletteChange?.(sampleMediaPalette(image))}
          onArtworkError={() => onPaletteChange?.(null)}
        />
        <div className="media-hero-copy">
          <p className="media-type-label">{isMovie ? '电影' : '剧集'}</p>
          <h1>{title}</h1>
          <div className="media-hero-facts">
            {originalTitle ? <span className="media-hero-original-title">{originalTitle}</span> : null}
            {releaseDate ? <span className="media-hero-year">{releaseDate.slice(0, 4)}</span> : null}
            {runtime > 0 ? <span className="media-hero-runtime">{runtime} 分钟</span> : null}
            {genres.map((genre) => <span className="media-hero-genre" key={genre}>{genre}</span>)}
          </div>
          {record.rating !== null ? (
            <span className="media-hero-rating"><Star aria-hidden="true" size={17} />{record.rating.toFixed(1)} / 10</span>
          ) : null}
          <p className="media-hero-overview">{overview}</p>
          <a className="media-hero-record-anchor" href="#personal-record">
            {record.status === 'none' ? '开始记录' : '更新记录'}
          </a>
          {linker}
        </div>
      </div>
    </header>
  )
}
