import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useMemo, useState } from 'react'

import { getCurrentRound, getEpisodeProgress, getTMDBSeason, getTMDBTV } from '../../api/client'
import type { CurrentRound, HouseholdMember } from '../../api/types'
import { RewatchSection } from '../records/RewatchSection'
import { RoundRecordForm } from '../records/RoundRecordForm'
import { EpisodeProgress } from './EpisodeProgress'
import { regularSeasons, selectActiveSeason, totalEpisodeCount } from './episodeCatalog'

type SeasonRecordWorkspaceProps = {
  mediaId: string
  tmdbId: number | null
  participants: HouseholdMember[]
  now?: () => Date
  organizing?: (profileVersion: number, activeRound: CurrentRound) => ReactNode
}

export function SeasonRecordWorkspace({
  mediaId,
  tmdbId,
  participants,
  now = () => new Date(),
  organizing,
}: SeasonRecordWorkspaceProps) {
  const queryClient = useQueryClient()
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null)
  const tv = useQuery({
    queryKey: ['tmdb-tv', tmdbId],
    queryFn: ({ signal }) => getTMDBTV(tmdbId ?? 0, signal),
    enabled: tmdbId !== null,
  })
  const seasons = regularSeasons(tv.data?.seasons ?? [])
  const rounds = useQueries({
    queries: seasons.map((season) => ({
      queryKey: ['current-round', mediaId, season.seasonNumber],
      queryFn: ({ signal }: { signal: AbortSignal }) => getCurrentRound(mediaId, season.seasonNumber, signal),
    })),
  })
  const progressQueries = useQueries({
    queries: seasons.map((season) => ({
      queryKey: ['episode-progress', mediaId, season.seasonNumber],
      queryFn: ({ signal }: { signal: AbortSignal }) => getEpisodeProgress(mediaId, season.seasonNumber, signal),
      staleTime: 30_000,
    })),
  })
  const roundsPending = rounds.some((round) => round.isPending)
  const defaultSeason = roundsPending
    ? null
    : selectActiveSeason(seasons, rounds.flatMap((round) => round.data ? [round.data] : []))
  const activeSeason = selectedSeason ?? defaultSeason
  const activeRound = rounds.find((_, index) => seasons[index]?.seasonNumber === activeSeason)
  const season = useQuery({
    queryKey: ['tmdb-season', tmdbId, activeSeason],
    queryFn: ({ signal }) => getTMDBSeason(tmdbId ?? 0, activeSeason ?? 0, signal),
    enabled: tmdbId !== null && activeSeason !== null,
    staleTime: 30_000,
  })

  const seriesProgress = useMemo(() => {
    const total = totalEpisodeCount(seasons)
    const watched = seasons.reduce((sum, seasonSummary, index) => {
      const progress = progressQueries[index]?.data
      if (!progress) return sum
      const listed = progress.episodes.filter((episode) => episode.watched).length
      const reported = progress.watchedEpisodes ?? 0
      const seasonWatched = Math.max(listed, reported)
      return sum + Math.min(seasonWatched, seasonSummary.episodeCount || seasonWatched)
    }, 0)
    return { watched, total }
  }, [progressQueries, seasons])

  const firstIncompleteSeason = useMemo(() => {
    for (let index = 0; index < seasons.length; index += 1) {
      const seasonSummary = seasons[index]!
      const progress = progressQueries[index]?.data
      const listed = progress?.episodes.filter((episode) => episode.watched).length ?? 0
      const reported = progress?.watchedEpisodes ?? 0
      const watched = Math.max(listed, reported)
      if (watched < (seasonSummary.episodeCount || Number.MAX_SAFE_INTEGER)) {
        return seasonSummary.seasonNumber
      }
    }
    return null
  }, [progressQueries, seasons])

  if (tmdbId === null) {
    return (
      <section className="details-section season-workspace-error" role="alert">
        <h2>剧集记录</h2>
        <p>关联 TMDB 后可按季记录分集进度和个人记录。</p>
      </section>
    )
  }
  if (tv.isPending) return <SeasonWorkspaceSkeleton />
  if (tv.isError || !tv.data) {
    return (
      <section className="details-section season-workspace-error" role="alert">
        <h2>剧集记录</h2>
        <p>无法获取季资料，请稍后重试。</p>
      </section>
    )
  }
  if (roundsPending) return <SeasonWorkspaceSkeleton />
  if (seasons.length === 0 || activeSeason === null) {
    return (
      <section className="details-section season-workspace-error">
        <h2>剧集记录</h2>
        <p className="quiet-empty">TMDB 暂时没有常规季资料</p>
      </section>
    )
  }

  return (
    <section className="season-record-workspace" aria-labelledby="season-workspace-heading">
      <div className="season-workspace-toolbar">
        <div>
          <h2 id="season-workspace-heading">按季记录</h2>
          <p className="season-workspace-summary">
            全剧进度 {seriesProgress.total > 0
              ? `${seriesProgress.watched} / ${seriesProgress.total} 集`
              : `共 ${seasons.length} 季`}
            {firstIncompleteSeason !== null && firstIncompleteSeason !== activeSeason ? (
              <>
                {' · '}
                <button
                  type="button"
                  className="text-link-button"
                  onClick={() => setSelectedSeason(firstIncompleteSeason)}
                >
                  跳到未完成季
                </button>
              </>
            ) : null}
          </p>
        </div>
        <div className="season-chip-list" role="tablist" aria-label="选择季">
          {seasons.map((seasonSummary, index) => {
            const progress = progressQueries[index]?.data
            const listed = progress?.episodes.filter((episode) => episode.watched).length ?? 0
            const reported = progress?.watchedEpisodes ?? 0
            const watched = Math.min(Math.max(listed, reported), seasonSummary.episodeCount || Number.MAX_SAFE_INTEGER)
            const total = seasonSummary.episodeCount || progress?.totalEpisodes || 0
            const selected = seasonSummary.seasonNumber === activeSeason
            return (
              <button
                key={seasonSummary.id}
                type="button"
                role="tab"
                aria-selected={selected}
                className={selected ? 'is-selected' : ''}
                onClick={() => setSelectedSeason(seasonSummary.seasonNumber)}
              >
                {seasonSummary.name || `第 ${seasonSummary.seasonNumber} 季`}
                <small>{total > 0 ? `${watched}/${total}` : `${seasonSummary.episodeCount} 集`}</small>
              </button>
            )
          })}
        </div>
      </div>

      <div className="season-workspace-layout">
        <div className="season-workspace-primary">
          <EpisodeProgress
            mediaId={mediaId}
            tmdbId={tmdbId}
            seasonNumber={activeSeason}
            now={now}
          />
        </div>
        <aside className="personal-record-panel season-record-panel">
          {activeRound?.isPending ? <SeasonRecordSkeleton /> : null}
          {activeRound?.isError ? <p className="form-message error" role="alert">无法读取本季个人记录。</p> : null}
          {activeRound?.data ? (
            <>
              <RoundRecordForm
                round={activeRound.data}
                now={now()}
                participants={participants}
                onSaved={(saved) => queryClient.setQueryData(
                  ['current-round', mediaId, activeSeason],
                  saved,
                )}
              />
              {organizing?.(activeRound.data.profileVersion, activeRound.data)}
            </>
          ) : null}
        </aside>
      </div>
      {activeRound?.data ? (
        <RewatchSection
          round={activeRound.data}
          episodeCatalog={season.data?.episodes ?? []}
          onRewatched={(saved) => queryClient.setQueryData(
            ['current-round', mediaId, activeSeason],
            saved,
          )}
        />
      ) : null}
    </section>
  )
}

function SeasonWorkspaceSkeleton() {
  return (
    <section className="season-record-workspace" aria-label="正在加载季记录">
      <div className="skeleton season-workspace-toolbar-skeleton" />
      <div className="skeleton season-workspace-content-skeleton" />
    </section>
  )
}

function SeasonRecordSkeleton() {
  return <div className="skeleton season-record-skeleton" aria-label="正在加载本季个人记录" />
}
