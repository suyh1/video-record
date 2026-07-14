import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ChevronRight, Circle, LoaderCircle, RotateCcw } from 'lucide-react'
import { useState } from 'react'

import {
  getEpisodeProgress,
  getTMDBSeason,
  getTMDBTV,
  updateEpisodeProgress,
  type UpdateEpisodeProgressPayload,
} from '../../api/client'
import type { EpisodeProgressItem, EpisodeReference, SeriesProgress } from '../../api/types'
import {
  findNextEpisode,
  mergeSeason,
  regularSeasons,
  selectDefaultSeason,
  totalEpisodeCount,
  type MergedEpisode,
} from './episodeCatalog'

type EpisodeProgressProps = {
  mediaId: string
  tmdbId?: number | null
}

export function EpisodeProgress({ mediaId, tmdbId = null }: EpisodeProgressProps) {
  const progress = useQuery({
    queryKey: ['episode-progress', mediaId],
    queryFn: ({ signal }) => getEpisodeProgress(mediaId, signal),
  })

  if (progress.isPending) return <EpisodeProgressSkeleton />
  if (progress.isError) {
    return (
      <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
        <h2 id="episode-progress-heading">剧集进度</h2>
        <p className="episode-progress-error" role="alert">无法读取剧集进度，请稍后重试。</p>
      </section>
    )
  }
  if (tmdbId) return <LiveEpisodeProgress mediaId={mediaId} tmdbId={tmdbId} progress={progress.data} />
  return <LegacyEpisodeProgress mediaId={mediaId} progress={progress.data} />
}

function LiveEpisodeProgress({ mediaId, tmdbId, progress }: {
  mediaId: string
  tmdbId: number
  progress: SeriesProgress
}) {
  const queryClient = useQueryClient()
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null)
  const [rangeStart, setRangeStart] = useState('')
  const [rangeEnd, setRangeEnd] = useState('')
  const [savedAction, setSavedAction] = useState<{ label: string; episode: EpisodeReference } | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const tv = useQuery({
    queryKey: ['tmdb-tv', tmdbId],
    queryFn: ({ signal }) => getTMDBTV(tmdbId, signal),
  })
  const defaultSeason = tv.data ? selectDefaultSeason(tv.data.seasons, progress) : null
  const activeSeason = selectedSeason ?? defaultSeason
  const season = useQuery({
    queryKey: ['tmdb-season', tmdbId, activeSeason],
    queryFn: ({ signal }) => getTMDBSeason(tmdbId, activeSeason ?? 0, signal),
    enabled: activeSeason !== null,
  })
  const mutation = useMutation({
    mutationFn: (payload: UpdateEpisodeProgressPayload) => updateEpisodeProgress(mediaId, payload),
    onSuccess: (nextProgress, variables) => {
      queryClient.setQueryData(['episode-progress', mediaId], nextProgress)
      setErrorMessage('')
      const savedEpisode = variables.episodeRefs?.[0]
      if (variables.action === 'next' && savedEpisode) {
        setSavedAction({ label: episodeLabel(savedEpisode), episode: savedEpisode })
      } else if (variables.action === 'undo') {
        setSavedAction(null)
      }
    },
    onError: () => setErrorMessage('进度保存失败，请稍后重试。'),
  })

  const totalEpisodes = tv.data ? totalEpisodeCount(tv.data.seasons) : progress.totalEpisodes
  const watchedEpisodes = progress.episodes.filter((episode) => episode.watched).length
  const submit = (action: UpdateEpisodeProgressPayload['action'], episodes: EpisodeReference[]) => {
    mutation.mutate({
      action,
      expectedVersion: progress.version,
      episodeRefs: episodes,
      totalEpisodes,
      ...(action === 'undo' ? {} : { watchedAt: new Date().toISOString() }),
    })
  }

  if (tv.isPending) return <EpisodeProgressSkeleton />
  if (tv.isError) {
    return (
      <LiveCatalogError
        message="无法获取最新剧集资料"
        watchedEpisodes={watchedEpisodes}
        onRetry={() => void tv.refetch()}
      />
    )
  }
  const availableSeasons = regularSeasons(tv.data.seasons)
  if (availableSeasons.length === 0 || activeSeason === null) {
    return (
      <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
        <h2 id="episode-progress-heading">剧集进度</h2>
        <p className="quiet-empty">TMDB 暂时没有常规季资料</p>
      </section>
    )
  }
  if (season.isPending) return <EpisodeProgressSkeleton />
  if (season.isError) {
    return (
      <LiveCatalogError
        message={`无法获取第 ${activeSeason} 季分集资料`}
        watchedEpisodes={watchedEpisodes}
        onRetry={() => void season.refetch()}
      />
    )
  }

  const merged = mergeSeason(season.data, tv.data.seasons, progress)
  const nextEpisode = findNextEpisode(merged.episodes)
  const complete = merged.episodes.length > 0 && merged.episodes.every((episode) => episode.watched)
  const reference = (episode: MergedEpisode): EpisodeReference => ({
    sourceId: String(episode.id),
    seasonNumber: episode.seasonNumber,
    episodeNumber: episode.episodeNumber,
    absoluteNumber: episode.absoluteNumber,
  })
  const rangeStartIndex = merged.episodes.findIndex((episode) => String(episode.id) === rangeStart)
  const rangeEndIndex = merged.episodes.findIndex((episode) => String(episode.id) === rangeEnd)
  const rangeValid = rangeStartIndex >= 0 && rangeEndIndex >= rangeStartIndex

  return (
    <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
      <div className="details-section-heading episode-progress-heading">
        <div>
          <h2 id="episode-progress-heading">剧集进度</h2>
          <p>{watchedEpisodes} / {totalEpisodes} 集</p>
        </div>
        {nextEpisode ? (
          <button
            className="episode-next-button"
            type="button"
            disabled={mutation.isPending}
            aria-label={`推进下一集 ${episodeLabel(nextEpisode)}`}
            onClick={() => submit('next', [reference(nextEpisode)])}
          >
            {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <ChevronRight aria-hidden="true" size={16} />}
            下一集
          </button>
        ) : <span className="episode-complete-label"><Check aria-hidden="true" size={16} />本季已看完</span>}
      </div>

      <progress className="episode-progress-meter" aria-label={`已看 ${watchedEpisodes} 集，共 ${totalEpisodes} 集`} value={watchedEpisodes} max={Math.max(totalEpisodes, 1)} />
      <label className="episode-season-select">
        <span>选择季</span>
        <select
          aria-label="选择季"
          value={activeSeason}
          onChange={(event) => {
            setSelectedSeason(Number(event.target.value))
            setRangeStart('')
            setRangeEnd('')
          }}
        >
          {availableSeasons.map((item) => (
            <option key={item.id} value={item.seasonNumber}>{item.name || `第 ${item.seasonNumber} 季`} · {item.episodeCount} 集</option>
          ))}
        </select>
      </label>

      {errorMessage ? <p className="episode-progress-error" role="alert">{errorMessage}</p> : null}
      {savedAction ? (
        <div className="episode-progress-toast" role="status">
          <span>已推进至 {savedAction.label}</span>
          <button type="button" disabled={mutation.isPending} onClick={() => submit('undo', [savedAction.episode])}>
            <RotateCcw aria-hidden="true" size={15} />撤销 {savedAction.label}
          </button>
        </div>
      ) : null}

      <details className="episode-range-controls">
        <summary>批量记录</summary>
        <div>
          <label>
            <span>从</span>
            <select value={rangeStart} onChange={(event) => setRangeStart(event.target.value)}>
              <option value="">选择起点</option>
              {merged.episodes.map((episode) => <option key={episode.id} value={episode.id}>{episodeLabel(episode)}</option>)}
            </select>
          </label>
          <label>
            <span>到</span>
            <select value={rangeEnd} onChange={(event) => setRangeEnd(event.target.value)}>
              <option value="">选择终点</option>
              {merged.episodes.map((episode) => <option key={episode.id} value={episode.id}>{episodeLabel(episode)}</option>)}
            </select>
          </label>
          <button
            type="button"
            disabled={mutation.isPending || !rangeValid}
            onClick={() => submit('range', merged.episodes.slice(rangeStartIndex, rangeEndIndex + 1).map(reference))}
          >标记范围已看</button>
          <button
            type="button"
            disabled={mutation.isPending || complete}
            onClick={() => submit('season', merged.episodes.map(reference))}
          >{complete ? '本季已看完' : '标记整季'}</button>
        </div>
      </details>

      <ul className="episode-list live-episode-list">
        {merged.episodes.map((episode) => (
          <li key={episode.id} className={episode.watched ? 'watched' : ''}>
            <button
              type="button"
              aria-pressed={episode.watched}
              aria-label={episode.watched ? `将 ${episodeLabel(episode)} 标为未看` : `标记 ${episodeLabel(episode)} 已看`}
              disabled={mutation.isPending}
              onClick={() => submit(episode.watched ? 'undo' : 'single', [reference(episode)])}
            >
              {episode.watched ? <Check aria-hidden="true" size={16} /> : <Circle aria-hidden="true" size={16} />}
              <span className="episode-code">{episodeLabel(episode)}</span>
              <span className="episode-absolute">全剧第 {episode.absoluteNumber} 集</span>
              <strong>{episode.name || '未命名'}</strong>
              <span className="episode-watch-state">{episode.watched ? '已看' : '未看'}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

function LiveCatalogError({ message, watchedEpisodes, onRetry }: {
  message: string
  watchedEpisodes: number
  onRetry: () => void
}) {
  return (
    <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
      <h2 id="episode-progress-heading">剧集进度</h2>
      <p className="episode-progress-error" role="alert">{message}</p>
      <p className="quiet-empty">已记录 {watchedEpisodes} 集</p>
      <button className="episode-catalog-retry" type="button" onClick={onRetry}>重新获取分集资料</button>
    </section>
  )
}

function LegacyEpisodeProgress({ mediaId, progress }: { mediaId: string; progress: SeriesProgress }) {
  const queryClient = useQueryClient()
  const [errorMessage, setErrorMessage] = useState('')
  const mutation = useMutation({
    mutationFn: (payload: UpdateEpisodeProgressPayload) => updateEpisodeProgress(mediaId, payload),
    onSuccess: (nextProgress) => {
      queryClient.setQueryData(['episode-progress', mediaId], nextProgress)
      setErrorMessage('')
    },
    onError: () => setErrorMessage('进度保存失败，请稍后重试。'),
  })
  if (progress.episodes.length === 0) {
    return (
      <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
        <h2 id="episode-progress-heading">剧集进度</h2>
        <p className="quiet-empty">关联 TMDB 后可按季记录分集进度</p>
      </section>
    )
  }
  const submit = (episode: EpisodeProgressItem) => mutation.mutate({
    action: episode.watched ? 'undo' : 'single',
    episodeId: episode.id,
    expectedVersion: progress.version,
    ...(episode.watched ? {} : { watchedAt: new Date().toISOString() }),
  })
  return (
    <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
      <div className="details-section-heading episode-progress-heading">
        <div><h2 id="episode-progress-heading">剧集进度</h2><p>{progress.watchedEpisodes} / {progress.totalEpisodes} 集</p></div>
      </div>
      {errorMessage ? <p className="episode-progress-error" role="alert">{errorMessage}</p> : null}
      <ul className="episode-list">
        {progress.episodes.map((episode) => (
          <li key={episode.id} className={episode.watched ? 'watched' : ''}>
            <button
              type="button"
              aria-pressed={episode.watched}
              aria-label={episode.watched ? `将 ${episodeLabel(episode)} 标为未看` : `标记 ${episodeLabel(episode)} 已看`}
              disabled={mutation.isPending}
              onClick={() => submit(episode)}
            >
              {episode.watched ? <Check aria-hidden="true" size={16} /> : <Circle aria-hidden="true" size={16} />}
              <span className="episode-code">{episodeLabel(episode)}</span>
              <span className="episode-absolute">全剧第 {episode.absoluteNumber} 集</span>
              <strong>{episode.name || '未命名'}</strong>
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

function EpisodeProgressSkeleton() {
  return (
    <section className="details-section episode-progress" aria-label="正在加载剧集进度">
      <div className="skeleton episode-progress-title-skeleton" />
      <div className="skeleton episode-progress-list-skeleton" />
    </section>
  )
}

function episodeLabel(episode: Pick<EpisodeProgressItem, 'seasonNumber' | 'episodeNumber'>) {
  return `S${String(episode.seasonNumber).padStart(2, '0')}E${String(episode.episodeNumber).padStart(2, '0')}`
}
