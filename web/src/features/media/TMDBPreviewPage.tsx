import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bookmark, Check, CircleStop, LoaderCircle, Play } from 'lucide-react'
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import {
  createMediaFromTMDB,
  getCurrentRound,
  getTMDBCredits,
  getTMDBMovie,
  getTMDBTV,
  updateCurrentRound,
  type UpdateCurrentRoundPayload,
} from '../../api/client'
import type {
  MediaDetails,
  MediaSearchResult,
  MediaType,
  RecordStatus,
  RecordState,
  TMDBMovieDetails,
  TMDBTVDetails,
} from '../../api/types'
import { CastStrip } from './CastStrip'
import { MediaHero } from './MediaHero'
import { useMediaAtmosphere } from './mediaAtmosphere'

const emptyRecord: RecordState = {
  mediaId: '',
  status: 'none',
  rating: null,
  note: null,
  watchedAt: null,
  viewingMethod: null,
  version: 0,
}

const movieStatusOptions = [
  { value: 'wishlist', label: '想看', icon: Bookmark },
  { value: 'watching', label: '在看', icon: Play },
  { value: 'completed', label: '看过', icon: Check },
  { value: 'dropped', label: '弃看', icon: CircleStop },
] as const

export function TMDBPreviewPage() {
  const { mediaType: rawMediaType = '', tmdbId: rawTMDBID = '' } = useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [selectedStatus, setSelectedStatus] = useState<RecordStatus | null>(null)
  const atmosphere = useMediaAtmosphere(`${rawMediaType}:${rawTMDBID}`)
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
  const media = valid && details.data ? previewMedia(mediaType, tmdbID, details.data) : null
  const searchResult = media ? previewSearchResult(media) : null
  const materialize = useMutation({
    mutationFn: async (status: RecordStatus | null) => {
      if (!searchResult) throw new Error('TMDB preview is not ready')
      const imported = await createMediaFromTMDB(searchResult)
      if (searchResult.mediaType === 'movie') {
        if (!status || status === 'none') throw new Error('Movie status required')
        const payload: UpdateCurrentRoundPayload = { status }
        if (status === 'completed') payload.watchedAt = new Date().toISOString()
        await updateCurrentRound(imported.id, undefined, 0, payload)
      } else if (status === 'wishlist') {
        const current = await getCurrentRound(imported.id, 1)
        await updateCurrentRound(imported.id, 1, current.version, { status: 'wishlist' })
      }
      return imported
    },
    onSuccess: (imported) => {
      void queryClient.invalidateQueries({ queryKey: ['media-search'] })
      void queryClient.invalidateQueries({ queryKey: ['library'] })
      navigate(`/media/${imported.id}`, { replace: true })
    },
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

  if (!media) return <PreviewSkeleton />
  const record = { ...emptyRecord, mediaId: media.id }

  return (
    <div className="page media-details-page tmdb-preview-page media-atmosphere-page" style={atmosphere.style}>
      <MediaHero
        media={media}
        record={record}
        linker={<span className="tmdb-preview-label">TMDB 预览</span>}
        onPaletteChange={atmosphere.onPaletteChange}
      />
      {credits.data?.crew && credits.data.crew.length > 0 ? (
        <p className="media-crew-line" aria-label="主创">
          {credits.data.crew.map((member) => `${member.job} ${member.name}`).join(' · ')}
        </p>
      ) : null}
      <CastStrip
        cast={credits.data?.cast ?? []}
        pending={credits.isPending}
        error={credits.isError}
        linked
        onRetry={() => void credits.refetch()}
      />
      <section className="tmdb-preview-record" aria-labelledby="tmdb-preview-record-heading">
        <div>
          <h2 id="tmdb-preview-record-heading">记录到我的影库</h2>
          <p>{mediaType === 'movie'
            ? '选择状态并保存后，这个条目才会进入本地影库。'
            : '开始记录后进入季与分集工作区；仅查看预览不会保存。'}</p>
        </div>
        {mediaType === 'movie' ? (
          <div className="tmdb-preview-movie-record">
            <fieldset>
              <legend>观看状态</legend>
              <div className="status-control" role="radiogroup" aria-label="观看状态">
                {movieStatusOptions.map(({ value, label, icon: Icon }) => (
                  <button
                    key={value}
                    className={selectedStatus === value ? 'selected' : ''}
                    type="button"
                    role="radio"
                    aria-checked={selectedStatus === value}
                    disabled={materialize.isPending}
                    onClick={() => {
                      setSelectedStatus(value)
                      materialize.reset()
                    }}
                  >
                    <Icon aria-hidden="true" size={16} />
                    {label}
                  </button>
                ))}
              </div>
            </fieldset>
            <button
              className="primary-button"
              type="button"
              disabled={!selectedStatus || materialize.isPending}
              onClick={() => materialize.mutate(selectedStatus)}
            >
              {materialize.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Check aria-hidden="true" size={16} />}
              保存记录
            </button>
          </div>
        ) : (
          <div className="tmdb-preview-tv-actions">
            <button
              className="primary-button"
              type="button"
              disabled={materialize.isPending}
              onClick={() => materialize.mutate('wishlist')}
            >
              {materialize.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Bookmark aria-hidden="true" size={16} />}
              标为想看并入库
            </button>
            <button
              className="secondary-button"
              type="button"
              disabled={materialize.isPending}
              onClick={() => materialize.mutate(null)}
            >
              {materialize.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Play aria-hidden="true" size={16} />}
              开始记录
            </button>
          </div>
        )}
        {materialize.isError ? (
          <p className="form-message error" role="alert">保存失败，请检查连接后重试。你的选择仍保留在此处。</p>
        ) : null}
      </section>
    </div>
  )
}

function previewSearchResult(media: MediaDetails): MediaSearchResult {
  if (media.tmdbId === null) throw new Error('TMDB identity required')
  return {
    id: media.id,
    externalId: media.tmdbId,
    source: 'tmdb',
    mediaType: media.mediaType,
    title: media.title,
    originalTitle: media.originalTitle,
    year: media.releaseDate.slice(0, 4),
    posterPath: media.posterPath,
    status: 'none',
  }
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
