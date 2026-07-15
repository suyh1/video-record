import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, LoaderCircle, Repeat2, X } from 'lucide-react'
import { useId, useState } from 'react'

import { getRoundDetail, getRoundHistory, startRewatch } from '../../api/client'
import type {
  ArchivedRound,
  CurrentRound,
  RoundSummary,
  SeriesProgress,
  TMDBEpisodeDetails,
} from '../../api/types'
import { formatLocalSeconds } from '../../lib/dateTime'

type EpisodeTitle = Pick<TMDBEpisodeDetails, 'seasonNumber' | 'episodeNumber' | 'name'>

type RewatchSectionProps = {
  round: CurrentRound
  episodeCatalog?: readonly EpisodeTitle[]
  onRewatched?: (round: CurrentRound) => void
}

export function RewatchSection({ round, episodeCatalog = [], onRewatched }: RewatchSectionProps) {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<RoundSummary | null>(null)
  const [errorMessage, setErrorMessage] = useState('')
  const headingID = useId()
  const disabledReasonID = useId()
  const seasonNumber = round.seasonNumber ?? undefined
  const scopeKey = round.seasonNumber ?? 'movie'
  const historyKey = ['round-history', round.mediaId, scopeKey] as const
  const currentRoundKey = ['current-round', round.mediaId, scopeKey] as const
  const history = useQuery({
    queryKey: historyKey,
    queryFn: ({ signal }) => getRoundHistory(round.mediaId, seasonNumber, signal),
  })
  const detail = useQuery({
    queryKey: ['round-detail', round.mediaId, scopeKey, selected?.roundId],
    queryFn: ({ signal }) => getRoundDetail(round.mediaId, selected?.roundId ?? '', seasonNumber, signal),
    enabled: selected !== null,
  })
  const mutation = useMutation({
    mutationFn: () => startRewatch(round.mediaId, seasonNumber, round.version),
    onSuccess: (result) => {
      queryClient.setQueryData(currentRoundKey, result.current)
      queryClient.setQueryData<RoundSummary[]>(historyKey, (current) => [
        archivedSummary(result.archived),
        ...(current ?? []),
      ])
      if (round.seasonNumber !== null) {
        queryClient.setQueryData<SeriesProgress>(
          ['episode-progress', round.mediaId, round.seasonNumber],
          (current) => ({
            roundId: result.current.roundId,
            mediaId: round.mediaId,
            seasonNumber: round.seasonNumber as number,
            status: result.current.status,
            version: result.current.version,
            watchedEpisodes: 0,
            totalEpisodes: current?.totalEpisodes ?? 0,
            lastWatched: null,
            nextEpisode: null,
            episodes: [],
          }),
        )
      }
      setErrorMessage('')
      onRewatched?.(result.current)
      void queryClient.invalidateQueries({ queryKey: ['record', round.mediaId] })
      void queryClient.invalidateQueries({ queryKey: ['library'] })
    },
    onError: () => setErrorMessage('再刷失败，当前记录和多刷历史均未更改。'),
  })
  const canRewatch = round.status === 'completed'
  const rounds = history.data ?? []
  const episodeTitles = new Map(episodeCatalog.map((episode) => [
    episodeTitleKey(episode.seasonNumber, episode.episodeNumber),
    episode.name,
  ]))

  return (
    <section className="details-section rewatch-section" aria-labelledby={headingID}>
      <div className="details-section-heading rewatch-heading">
        <div>
          <h2 id={headingID}>多刷</h2>
          <p>{rounds.length} 次已归档</p>
        </div>
        <button
          className="rewatch-button"
          type="button"
          disabled={!canRewatch || mutation.isPending}
          aria-describedby={!canRewatch ? disabledReasonID : undefined}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending
            ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
            : <Repeat2 aria-hidden="true" size={16} />}
          {mutation.isPending ? '正在开始' : '再刷'}
        </button>
      </div>
      {!canRewatch ? <p id={disabledReasonID} className="rewatch-availability">当前一刷完成后可再刷</p> : null}
      {errorMessage ? <p className="form-message error" role="alert">{errorMessage}</p> : null}
      {history.isPending ? <div className="skeleton rewatch-list-skeleton" aria-label="正在加载多刷记录" /> : null}
      {history.isError ? <p className="episode-progress-error" role="alert">无法读取多刷记录。</p> : null}
      {!history.isPending && !history.isError && rounds.length === 0 ? <p className="quiet-empty">暂无多刷记录</p> : null}
      {rounds.length > 0 ? (
        <ol className="rewatch-list">
          {rounds.map((item) => (
            <li key={item.roundId}>
              <div className="rewatch-summary">
                <strong>第 {item.roundNumber} 刷</strong>
                <span>{displayTime(item.watchedAt)}</span>
              </div>
              <span className="rewatch-rating">{item.rating === null ? '未评分' : `${item.rating.toFixed(1)} / 10`}</span>
              <button type="button" aria-label={`查看第 ${item.roundNumber} 刷`} onClick={() => setSelected(item)}>
                <Eye aria-hidden="true" size={16} />查看
              </button>
            </li>
          ))}
        </ol>
      ) : null}

      <Dialog.Root open={selected !== null} onOpenChange={(open) => { if (!open) setSelected(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-backdrop" />
          <Dialog.Content className="member-dialog round-detail-dialog">
            <div className="round-detail-dialog-heading">
              <Dialog.Title>第 {selected?.roundNumber ?? ''} 刷记录</Dialog.Title>
              <Dialog.Close asChild>
                <button type="button" aria-label="关闭多刷详情"><X aria-hidden="true" size={18} /></button>
              </Dialog.Close>
            </div>
            <Dialog.Description>
              {selected?.watchedAt ? `完成于 ${displayTime(selected.watchedAt)}` : '未记录完成时间'}
            </Dialog.Description>
            {detail.isPending ? <div className="skeleton round-detail-skeleton" aria-label="正在加载多刷详情" /> : null}
            {detail.isError ? <p className="episode-progress-error" role="alert">无法读取该轮详情。</p> : null}
            {detail.data ? (
              <div className="round-detail-content">
                <dl className="round-detail-facts">
                  <div><dt>完成时间</dt><dd>{displayTime(detail.data.round.watchedAt)}</dd></div>
                  <div><dt>评分</dt><dd>{detail.data.round.rating === null ? '未评分' : `${detail.data.round.rating.toFixed(1)} / 10`}</dd></div>
                  <div><dt>观看方式</dt><dd>{detail.data.round.viewingMethod || '未记录'}</dd></div>
                  <div className="round-detail-note"><dt>私人笔记</dt><dd>{detail.data.round.note || '未记录'}</dd></div>
                </dl>
                {round.seasonNumber !== null && detail.data.episodes.length > 0 ? (
                  <ol className="round-detail-episodes">
                    {detail.data.episodes.map((episode) => (
                      <li key={episode.id}>
                        <strong>{episodeLabel(episode.seasonNumber, episode.episodeNumber)}</strong>
                        <span>{episodeTitles.get(episodeTitleKey(episode.seasonNumber, episode.episodeNumber)) || episode.name || '未命名'}</span>
                        <time>{displayTime(episode.watchedAt)}</time>
                      </li>
                    ))}
                  </ol>
                ) : null}
              </div>
            ) : null}
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </section>
  )
}

function archivedSummary(round: ArchivedRound): RoundSummary {
  return {
    roundId: round.roundId,
    mediaId: round.mediaId,
    seasonNumber: round.seasonNumber,
    roundNumber: round.roundNumber,
    watchedAt: round.watchedAt,
    rating: round.rating,
  }
}

function displayTime(value: string | null) {
  return value ? formatLocalSeconds(value) : '未记录'
}

function episodeLabel(seasonNumber: number, episodeNumber: number) {
  return `S${String(seasonNumber).padStart(2, '0')}E${String(episodeNumber).padStart(2, '0')}`
}

function episodeTitleKey(seasonNumber: number, episodeNumber: number) {
  return `${seasonNumber}:${episodeNumber}`
}
