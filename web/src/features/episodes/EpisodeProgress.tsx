import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ChevronRight, Circle, LoaderCircle, RotateCcw } from 'lucide-react'
import { useRef, useState } from 'react'

import {
  getEpisodeProgress,
  getTMDBSeason,
  getTMDBTV,
  updateEpisodeProgress,
  type UpdateEpisodeProgressPayload,
} from '../../api/client'
import type { CurrentRound, EpisodeReference, SeriesProgress } from '../../api/types'
import { formatLocalSeconds } from '../../lib/dateTime'
import { EpisodeTimeEditor } from './EpisodeTimeEditor'
import { findNextEpisode, mergeSeason, type MergedEpisode } from './episodeCatalog'

type EpisodeProgressProps = {
  mediaId: string
  tmdbId: number | null
  seasonNumber: number
  now?: () => Date
}

export function EpisodeProgress({ mediaId, tmdbId, seasonNumber, now = () => new Date() }: EpisodeProgressProps) {
  const queryClient = useQueryClient()
  const [rangeStart, setRangeStart] = useState('')
  const [rangeEnd, setRangeEnd] = useState('')
  const [savedAction, setSavedAction] = useState<{ label: string; episode: EpisodeReference } | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const [editing, setEditing] = useState<{ sourceID: string; now: Date } | null>(null)
  const [pendingEpisodeID, setPendingEpisodeID] = useState<string | null>(null)
  const timeButtonRefs = useRef(new Map<string, HTMLButtonElement>())
  const progress = useQuery({
    queryKey: ['episode-progress', mediaId, seasonNumber],
    queryFn: ({ signal }) => getEpisodeProgress(mediaId, seasonNumber, signal),
    staleTime: 30_000,
  })
  const tv = useQuery({
    queryKey: ['tmdb-tv', tmdbId],
    queryFn: ({ signal }) => getTMDBTV(tmdbId ?? 0, signal),
    enabled: tmdbId !== null,
  })
  const season = useQuery({
    queryKey: ['tmdb-season', tmdbId, seasonNumber],
    queryFn: ({ signal }) => getTMDBSeason(tmdbId ?? 0, seasonNumber, signal),
    enabled: tmdbId !== null,
    staleTime: 30_000,
  })
  const mutation = useMutation({
    mutationFn: ({ payload }: ProgressMutation) => updateEpisodeProgress(mediaId, seasonNumber, payload),
    onMutate: ({ sourceID }) => {
      setPendingEpisodeID(sourceID)
      setErrorMessage('')
    },
    onSuccess: (nextProgress, variables) => {
      queryClient.setQueryData(['episode-progress', mediaId, seasonNumber], nextProgress)
      queryClient.setQueryData<CurrentRound>(['current-round', mediaId, seasonNumber], (current) => current ? {
        ...current,
        roundId: nextProgress.roundId,
        status: nextProgress.status,
        version: nextProgress.version,
        watchedAt: nextProgress.status === 'completed' ? latestWatchedAt(nextProgress) : null,
      } : current)
      void queryClient.invalidateQueries({ queryKey: ['current-round', mediaId, seasonNumber] })
      void queryClient.invalidateQueries({ queryKey: ['library'] })
      setErrorMessage('')
      if (variables.payload.action === 'next' && variables.episode) {
        setSavedAction({ label: episodeLabel(variables.episode), episode: variables.episode })
      } else if (variables.payload.action === 'undo') {
        setSavedAction(null)
      }
      if (variables.payload.action === 'set_time') setEditing(null)
    },
    onError: (_error, variables) => {
      setErrorMessage(variables.payload.action === 'set_time'
        ? '观看时间保存失败，请稍后重试。'
        : '进度保存失败，请稍后重试。')
    },
    onSettled: () => setPendingEpisodeID(null),
  })

  if (progress.isPending || tv.isPending || season.isPending) return <EpisodeProgressSkeleton />
  if (progress.isError) return <ProgressError message="无法读取本季进度，请稍后重试。" />
  if (tmdbId === null) return <ProgressError message="关联 TMDB 后可按季记录分集进度。" />
  if (tv.isError || !tv.data) {
    return <CatalogError message="无法获取最新剧集资料" onRetry={() => void tv.refetch()} />
  }
  if (season.isError || !season.data) {
    return <CatalogError message={`无法获取第 ${seasonNumber} 季分集资料`} onRetry={() => void season.refetch()} />
  }

  const merged = mergeSeason(season.data, tv.data.seasons, progress.data)
  const totalEpisodes = merged.episodes.length
  const watchedEpisodes = merged.episodes.filter((episode) => episode.watched).length
  const nextEpisode = findNextEpisode(merged.episodes)
  const complete = totalEpisodes > 0 && watchedEpisodes === totalEpisodes
  const rangeStartIndex = merged.episodes.findIndex((episode) => String(episode.id) === rangeStart)
  const rangeEndIndex = merged.episodes.findIndex((episode) => String(episode.id) === rangeEnd)
  const rangeValid = rangeStartIndex >= 0 && rangeEndIndex >= rangeStartIndex

  const submit = (
    action: UpdateEpisodeProgressPayload['action'],
    episodes: MergedEpisode[],
    watchedAt?: string,
  ) => {
    const references = episodes.map(toEpisodeReference)
    const sourceID = String(episodes[0]?.id ?? action)
    const episode = references[0]
    mutation.mutate({
      sourceID,
      ...(episode ? { episode } : {}),
      payload: {
        action,
        expectedVersion: progress.data.version,
        episodeRefs: references,
        totalEpisodes,
        ...(action === 'undo' ? {} : { watchedAt: watchedAt ?? now().toISOString() }),
      },
    })
  }

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
            disabled={pendingEpisodeID === String(nextEpisode.id)}
            aria-label={`推进下一集 ${episodeLabel(nextEpisode)}`}
            onClick={() => submit('next', [nextEpisode])}
          >
            {pendingEpisodeID === String(nextEpisode.id)
              ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
              : <ChevronRight aria-hidden="true" size={16} />}
            下一集
          </button>
        ) : <span className="episode-complete-label"><Check aria-hidden="true" size={16} />本季已看完</span>}
      </div>

      <progress
        className="episode-progress-meter"
        aria-label={`本季已看 ${watchedEpisodes} 集，共 ${totalEpisodes} 集`}
        value={watchedEpisodes}
        max={Math.max(totalEpisodes, 1)}
      />

      {errorMessage && editing === null ? <p className="episode-progress-error" role="alert">{errorMessage}</p> : null}
      {savedAction ? (
        <div className="episode-progress-toast" role="status">
          <span>已推进至 {savedAction.label}</span>
          <button
            type="button"
            disabled={pendingEpisodeID === savedAction.episode.sourceId}
            onClick={() => {
              const episode = merged.episodes.find((item) => String(item.id) === savedAction.episode.sourceId)
              if (episode) submit('undo', [episode])
            }}
          >
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
            disabled={pendingEpisodeID === 'range' || !rangeValid}
            onClick={() => submit('range', merged.episodes.slice(rangeStartIndex, rangeEndIndex + 1))}
          >标记范围已看</button>
          <button
            type="button"
            disabled={pendingEpisodeID === 'season' || complete}
            onClick={() => submit('season', merged.episodes)}
          >{complete ? '本季已看完' : '标记整季'}</button>
        </div>
      </details>

      <ul className="episode-list live-episode-list">
        {merged.episodes.map((episode) => {
          const sourceID = String(episode.id)
          const label = episodeLabel(episode)
          const pending = pendingEpisodeID === sourceID
          const timeLabel = episode.watchedAt ? formatLocalSeconds(episode.watchedAt) : '设置观看时间'
          return (
            <li key={episode.id} className={episode.watched ? 'watched' : ''}>
              <div className="episode-row">
                <button
                  className="episode-toggle"
                  type="button"
                  aria-pressed={episode.watched}
                  aria-label={episode.watched ? `将 ${label} 标为未看` : `标记 ${label} 已看`}
                  disabled={pending}
                  onClick={() => submit(episode.watched ? 'undo' : 'single', [episode])}
                >
                  {pending
                    ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
                    : episode.watched
                      ? <Check aria-hidden="true" size={16} />
                      : <Circle aria-hidden="true" size={16} />}
                </button>
                <span className="episode-code">{label}</span>
                <span className="episode-absolute">全剧第 {episode.absoluteNumber} 集</span>
                <strong>{episode.name || '未命名'}</strong>
                <button
                  className="episode-time-button"
                  type="button"
                  ref={(element) => {
                    if (element) timeButtonRefs.current.set(sourceID, element)
                    else timeButtonRefs.current.delete(sourceID)
                  }}
                  aria-expanded={editing?.sourceID === sourceID}
                  aria-label={episode.watchedAt ? `修改 ${label} 观看时间，当前 ${timeLabel}` : `设置 ${label} 观看时间`}
                  disabled={pending}
                  onClick={() => {
                    setErrorMessage('')
                    setEditing({ sourceID, now: now() })
                  }}
                >
                  {timeLabel}
                </button>
              </div>
              {editing?.sourceID === sourceID ? (
                <EpisodeTimeEditor
                  episodeLabel={label}
                  watchedAt={episode.watchedAt}
                  now={editing.now}
                  pending={pending}
                  error={errorMessage}
                  returnFocusRef={{ current: timeButtonRefs.current.get(sourceID) ?? null }}
                  onConfirm={(value) => submit('set_time', [episode], value)}
                  onCancel={() => {
                    setEditing(null)
                    setErrorMessage('')
                  }}
                />
              ) : null}
            </li>
          )
        })}
      </ul>
    </section>
  )
}

type ProgressMutation = {
  sourceID: string
  episode?: EpisodeReference
  payload: UpdateEpisodeProgressPayload
}

function ProgressError({ message }: { message: string }) {
  return (
    <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
      <h2 id="episode-progress-heading">剧集进度</h2>
      <p className="episode-progress-error" role="alert">{message}</p>
    </section>
  )
}

function CatalogError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
      <h2 id="episode-progress-heading">剧集进度</h2>
      <p className="episode-progress-error" role="alert">{message}</p>
      <button className="episode-catalog-retry" type="button" onClick={onRetry}>重新获取分集资料</button>
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

function toEpisodeReference(episode: MergedEpisode): EpisodeReference {
  return {
    sourceId: String(episode.id),
    seasonNumber: episode.seasonNumber,
    episodeNumber: episode.episodeNumber,
    absoluteNumber: episode.absoluteNumber,
  }
}

function latestWatchedAt(progress: SeriesProgress) {
  return progress.episodes.reduce<string | null>((latest, episode) => {
    if (!episode.watchedAt) return latest
    if (!latest || new Date(episode.watchedAt).getTime() > new Date(latest).getTime()) return episode.watchedAt
    return latest
  }, null)
}

function episodeLabel(episode: Pick<MergedEpisode, 'seasonNumber' | 'episodeNumber'> | EpisodeReference) {
  return `S${String(episode.seasonNumber).padStart(2, '0')}E${String(episode.episodeNumber).padStart(2, '0')}`
}
