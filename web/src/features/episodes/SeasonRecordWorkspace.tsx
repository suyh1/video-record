import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useState } from 'react'

import { getCurrentRound, getTMDBSeason, getTMDBTV } from '../../api/client'
import type { CurrentRound, HouseholdMember } from '../../api/types'
import { RewatchSection } from '../records/RewatchSection'
import { RoundRecordForm } from '../records/RoundRecordForm'
import { EpisodeProgress } from './EpisodeProgress'
import { regularSeasons, selectActiveSeason } from './episodeCatalog'

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
            全剧进度 {rounds.flatMap((round) => round.data?.status === 'completed' ? [1] : []).length === seasons.length
              ? '已全部完成'
              : `进行中 · 共 ${seasons.length} 季`}
          </p>
        </div>
        <div className="season-chip-list" role="tablist" aria-label="选择季">
          {seasons.map((season, index) => {
            const round = rounds[index]?.data
            const selected = season.seasonNumber === activeSeason
            return (
              <button
                key={season.id}
                type="button"
                role="tab"
                aria-selected={selected}
                className={selected ? 'is-selected' : ''}
                onClick={() => setSelectedSeason(season.seasonNumber)}
              >
                {season.name || `第 ${season.seasonNumber} 季`}
                <small>{round?.status === 'completed' ? '已看完' : `${season.episodeCount} 集`}</small>
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
