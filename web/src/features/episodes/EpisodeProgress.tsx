import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ChevronRight, Circle, LoaderCircle, RotateCcw } from 'lucide-react'
import { useState } from 'react'

import {
  getEpisodeProgress,
  updateEpisodeProgress,
  type UpdateEpisodeProgressPayload,
} from '../../api/client'
import type { EpisodeProgressItem, SeriesProgress } from '../../api/types'

type EpisodeProgressProps = {
  mediaId: string
}

type SavedAction = {
  label: string
  episodeId: string
}

export function EpisodeProgress({ mediaId }: EpisodeProgressProps) {
  const queryClient = useQueryClient()
  const [rangeStart, setRangeStart] = useState('')
  const [rangeEnd, setRangeEnd] = useState('')
  const [savedAction, setSavedAction] = useState<SavedAction | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const progress = useQuery({
    queryKey: ['episode-progress', mediaId],
    queryFn: ({ signal }) => getEpisodeProgress(mediaId, signal),
  })
  const mutation = useMutation({
    mutationFn: (payload: UpdateEpisodeProgressPayload) => updateEpisodeProgress(mediaId, payload),
    onSuccess: (nextProgress, variables) => {
      queryClient.setQueryData(['episode-progress', mediaId], nextProgress)
      setErrorMessage('')
      if (variables.action === 'next' && nextProgress.lastWatched) {
        setSavedAction({ label: episodeLabel(nextProgress.lastWatched), episodeId: nextProgress.lastWatched.id })
      } else if (variables.action !== 'undo') {
        setSavedAction(null)
      }
    },
    onError: () => setErrorMessage('进度保存失败，请稍后重试。'),
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
  if (progress.data.episodes.length === 0) {
    return (
      <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
        <h2 id="episode-progress-heading">剧集进度</h2>
        <p className="quiet-empty">暂时没有可记录的分集资料</p>
      </section>
    )
  }

  const data = progress.data
  const nextEpisode = data.nextEpisode ?? data.episodes.find((episode) => !episode.watched) ?? null
  const submit = (payload: Omit<UpdateEpisodeProgressPayload, 'expectedVersion' | 'watchedAt'>) => {
    mutation.mutate({
      ...payload,
      expectedVersion: data.version,
      ...(payload.action === 'undo' ? {} : { watchedAt: new Date().toISOString() }),
    })
  }
  const undoSaved = () => {
    if (!savedAction) return
    submit({ action: 'undo', episodeId: savedAction.episodeId })
    setSavedAction(null)
  }

  return (
    <section className="details-section episode-progress" aria-labelledby="episode-progress-heading">
      <div className="details-section-heading episode-progress-heading">
        <div>
          <h2 id="episode-progress-heading">剧集进度</h2>
          <p>{data.watchedEpisodes} / {data.totalEpisodes} 集</p>
        </div>
        {nextEpisode ? (
          <button
            className="episode-next-button"
            type="button"
            disabled={mutation.isPending}
            aria-label={`推进下一集 ${episodeLabel(nextEpisode)}`}
            onClick={() => submit({ action: 'next' })}
          >
            {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <ChevronRight aria-hidden="true" size={16} />}
            下一集
          </button>
        ) : (
          <span className="episode-complete-label"><Check aria-hidden="true" size={16} />已全部看完</span>
        )}
      </div>

      {errorMessage ? <p className="episode-progress-error" role="alert">{errorMessage}</p> : null}
      {savedAction ? (
        <div className="episode-progress-toast" role="status">
          <span>已推进至 {savedAction.label}</span>
          <button type="button" disabled={mutation.isPending} onClick={undoSaved}>
            <RotateCcw aria-hidden="true" size={15} />
            撤销 {savedAction.label}
          </button>
        </div>
      ) : null}

      <EpisodeRangeControls
        episodes={data.episodes}
        rangeStart={rangeStart}
        rangeEnd={rangeEnd}
        disabled={mutation.isPending}
        onRangeStart={setRangeStart}
        onRangeEnd={setRangeEnd}
        onSubmit={() => submit({ action: 'range', episodeId: rangeStart, throughEpisodeId: rangeEnd })}
      />

      <div className="episode-seasons">
        {groupBySeason(data).map(({ seasonId, seasonNumber, episodes }) => {
          const complete = episodes.every((episode) => episode.watched)
          return (
            <section className="episode-season" key={seasonId} aria-labelledby={`season-${seasonId}`}>
              <div className="episode-season-heading">
                <h3 id={`season-${seasonId}`}>第 {seasonNumber} 季</h3>
                <button
                  type="button"
                  disabled={complete || mutation.isPending}
                  onClick={() => submit({ action: 'season', seasonId })}
                >
                  {complete ? '本季已看完' : '标记整季'}
                </button>
              </div>
              <ul className="episode-list">
                {episodes.map((episode) => (
                  <li key={episode.id} className={episode.watched ? 'watched' : ''}>
                    <button
                      type="button"
                      aria-pressed={episode.watched}
                      aria-label={episode.watched ? `将 ${episodeLabel(episode)} 标为未看` : `标记 ${episodeLabel(episode)} 已看`}
                      disabled={mutation.isPending}
                      onClick={() => submit({
                        action: episode.watched ? 'undo' : 'single',
                        episodeId: episode.id,
                      })}
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
        })}
      </div>
    </section>
  )
}

type EpisodeRangeControlsProps = {
  episodes: EpisodeProgressItem[]
  rangeStart: string
  rangeEnd: string
  disabled: boolean
  onRangeStart: (value: string) => void
  onRangeEnd: (value: string) => void
  onSubmit: () => void
}

function EpisodeRangeControls(props: EpisodeRangeControlsProps) {
  const startIndex = props.episodes.findIndex((episode) => episode.id === props.rangeStart)
  const rangeValid = startIndex >= 0 && props.episodes.findIndex((episode) => episode.id === props.rangeEnd) >= startIndex
  return (
    <details className="episode-range-controls">
      <summary>连续标记多集</summary>
      <div>
        <label>
          <span>从</span>
          <select value={props.rangeStart} onChange={(event) => props.onRangeStart(event.target.value)}>
            <option value="">选择起点</option>
            {props.episodes.map((episode) => <option key={episode.id} value={episode.id}>{episodeLabel(episode)}</option>)}
          </select>
        </label>
        <label>
          <span>到</span>
          <select value={props.rangeEnd} onChange={(event) => props.onRangeEnd(event.target.value)}>
            <option value="">选择终点</option>
            {props.episodes.map((episode) => <option key={episode.id} value={episode.id}>{episodeLabel(episode)}</option>)}
          </select>
        </label>
        <button type="button" disabled={props.disabled || !rangeValid} onClick={props.onSubmit}>标记为已看</button>
      </div>
    </details>
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

function groupBySeason(progress: SeriesProgress) {
  const groups = new Map<string, { seasonId: string; seasonNumber: number; episodes: EpisodeProgressItem[] }>()
  for (const episode of progress.episodes) {
    const group = groups.get(episode.seasonId) ?? {
      seasonId: episode.seasonId,
      seasonNumber: episode.seasonNumber,
      episodes: [],
    }
    group.episodes.push(episode)
    groups.set(episode.seasonId, group)
  }
  return [...groups.values()]
}

function episodeLabel(episode: EpisodeProgressItem) {
  return `S${String(episode.seasonNumber).padStart(2, '0')}E${String(episode.episodeNumber).padStart(2, '0')}`
}
