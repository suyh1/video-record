import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'

import { getTMDBCredits, getTMDBMovie, getTMDBTV } from '../../api/client'
import type {
  MediaDetails,
  MediaType,
  RecordState,
  TMDBMovieDetails,
  TMDBTVDetails,
} from '../../api/types'
import { CastStrip } from './CastStrip'
import { MediaHero } from './MediaHero'

const emptyRecord: RecordState = {
  mediaId: '',
  status: 'none',
  rating: null,
  note: null,
  watchedAt: null,
  viewingMethod: null,
  version: 0,
}

export function TMDBPreviewPage() {
  const { mediaType: rawMediaType = '', tmdbId: rawTMDBID = '' } = useParams()
  const mediaType = parseMediaType(rawMediaType)
  const tmdbID = parseTMDBID(rawTMDBID)
  const valid = mediaType !== null && tmdbID !== null
  const details = useQuery<TMDBMovieDetails | TMDBTVDetails>({
    queryKey: ['tmdb-preview', mediaType, tmdbID],
    queryFn: ({ signal }) => mediaType === 'movie'
      ? getTMDBMovie(tmdbID ?? 0, signal)
      : getTMDBTV(tmdbID ?? 0, signal),
    enabled: valid,
  })
  const credits = useQuery({
    queryKey: ['tmdb-credits', mediaType, tmdbID],
    queryFn: ({ signal }) => getTMDBCredits(mediaType ?? 'movie', tmdbID ?? 0, signal),
    enabled: valid,
  })

  if (!valid) {
    return (
      <div className="page page-error" role="alert">
        <h1>无法打开 TMDB 详情</h1>
        <p>影视类型或 TMDB 编号无效。</p>
      </div>
    )
  }
  if (details.isPending) return <PreviewSkeleton />
  if (details.isError || !details.data) {
    return (
      <div className="page page-error" role="alert">
        <h1>无法获取 TMDB 详情</h1>
        <p>本地影库没有发生变化，请检查连接后重试。</p>
        <button type="button" onClick={() => void details.refetch()}>重新加载详情</button>
      </div>
    )
  }

  const media = previewMedia(mediaType, tmdbID, details.data)
  const record = { ...emptyRecord, mediaId: media.id }

  return (
    <div className="page media-details-page tmdb-preview-page">
      <MediaHero
        media={media}
        record={record}
        linker={<span className="tmdb-preview-label">TMDB 预览</span>}
      />
      <CastStrip
        cast={credits.data ?? []}
        pending={credits.isPending}
        error={credits.isError}
        linked
        onRetry={() => void credits.refetch()}
      />
      <section className="tmdb-preview-record" aria-labelledby="tmdb-preview-record-heading">
        <div>
          <h2 id="tmdb-preview-record-heading">记录到我的影库</h2>
          <p>只有开始记录后，这个条目才会保存到本地影库。</p>
        </div>
        <button className="primary-button" type="button">开始记录</button>
      </section>
    </div>
  )
}

function parseMediaType(value: string): MediaType | null {
  return value === 'movie' || value === 'tv' ? value : null
}

function parseTMDBID(value: string): number | null {
  if (!/^\d+$/.test(value)) return null
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
}

function previewMedia(
  mediaType: MediaType,
  tmdbID: number,
  details: TMDBMovieDetails | TMDBTVDetails,
): MediaDetails {
  if (mediaType === 'movie' && 'title' in details) {
    return {
      id: `tmdb-movie-${tmdbID}`,
      tmdbId: tmdbID,
      mediaType,
      title: details.title,
      externalTitle: details.title,
      externalOverview: details.overview,
      originalTitle: details.originalTitle,
      releaseDate: details.releaseDate,
      overview: details.overview,
      posterPath: details.posterPath,
      backdropPath: details.backdropPath,
      runtimeMinutes: details.runtime,
      genres: details.genres,
    }
  }
  const tv = details as TMDBTVDetails
  return {
    id: `tmdb-tv-${tmdbID}`,
    tmdbId: tmdbID,
    mediaType,
    title: tv.name,
    externalTitle: tv.name,
    externalOverview: tv.overview,
    originalTitle: tv.originalName,
    releaseDate: tv.firstAirDate,
    overview: tv.overview,
    posterPath: tv.posterPath,
    backdropPath: tv.backdropPath,
    runtimeMinutes: tv.episodeRuntime[0] ?? 0,
    genres: tv.genres,
  }
}

function PreviewSkeleton() {
  return (
    <div className="page details-skeleton" aria-label="正在加载 TMDB 详情">
      <div className="skeleton poster-skeleton" />
      <div className="skeleton copy-skeleton" />
    </div>
  )
}
