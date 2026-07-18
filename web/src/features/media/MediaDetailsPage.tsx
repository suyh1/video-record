import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useParams } from 'react-router-dom'

import {
  getCurrentRound,
  getHouseholdParticipants,
  getMedia,
  getRecord,
  getTMDBCredits,
  getTMDBMovie,
  getTMDBTV,
} from '../../api/client'
import type { CurrentRound, RecordState, TMDBMovieDetails, TMDBTVDetails } from '../../api/types'
import { CollectionPicker } from '../collections/CollectionPicker'
import { SeasonRecordWorkspace } from '../episodes/SeasonRecordWorkspace'
import { HouseholdSharedRecords } from '../records/HouseholdSharedRecords'
import { RecordDangerZone } from '../records/RecordDangerZone'
import { RecordSharingEditor } from '../records/RecordSharingEditor'
import { RecordTagsEditor } from '../records/RecordTagsEditor'
import { RewatchSection } from '../records/RewatchSection'
import { RoundRecordForm } from '../records/RoundRecordForm'
import { CastStrip } from './CastStrip'
import { MediaHero } from './MediaHero'
import { MediaStills } from './MediaStills'
import { useMediaAtmosphere } from './mediaAtmosphere'
import { TMDBLinker } from './TMDBLinker'

export function MediaDetailsPage() {
  const { mediaId = '' } = useParams()
  const queryClient = useQueryClient()
  const atmosphere = useMediaAtmosphere(mediaId)
  const [organizingOpen, setOrganizingOpen] = useState(false)
  const media = useQuery({
    queryKey: ['media', mediaId],
    queryFn: ({ signal }) => getMedia(mediaId, signal),
    enabled: Boolean(mediaId),
  })
  const record = useQuery({
    queryKey: ['record', mediaId],
    queryFn: ({ signal }) => getRecord(mediaId, signal),
    enabled: Boolean(mediaId),
  })
  const movieRound = useQuery({
    queryKey: ['current-round', mediaId, 'movie'],
    queryFn: ({ signal }) => getCurrentRound(mediaId, undefined, signal),
    enabled: media.data?.mediaType === 'movie',
  })
  const participants = useQuery({
    queryKey: ['household-participants'],
    queryFn: ({ signal }) => getHouseholdParticipants(signal),
    enabled: Boolean(mediaId),
  })
  const tmdbID = media.data?.tmdbId ?? null
  const mediaType = media.data?.mediaType
  const external = useQuery<TMDBMovieDetails | TMDBTVDetails>({
    queryKey: ['tmdb-media', mediaType, tmdbID],
    queryFn: ({ signal }) => mediaType === 'movie'
      ? getTMDBMovie(tmdbID ?? 0, signal)
      : getTMDBTV(tmdbID ?? 0, signal),
    enabled: Boolean(tmdbID && mediaType),
  })
  const credits = useQuery({
    queryKey: ['tmdb-credits', mediaType, tmdbID],
    queryFn: ({ signal }) => getTMDBCredits(mediaType ?? 'movie', tmdbID ?? 0, signal),
    enabled: Boolean(tmdbID && mediaType),
  })

  const movieRoundPending = media.data?.mediaType === 'movie' && movieRound.isPending
  const movieRoundError = media.data?.mediaType === 'movie' && movieRound.isError
  if (media.isPending || record.isPending || movieRoundPending) return <DetailsSkeleton />
  if (media.isError || record.isError || movieRoundError) {
    return (
      <div className="page page-error" role="alert">
        <h1>无法打开影视详情</h1>
        <p>记录仍保存在服务器中，请稍后重试。</p>
      </div>
    )
  }

  const updateProfileVersion = (version: number) => {
    queryClient.setQueryData<RecordState>(['record', mediaId], (current) => current ? { ...current, version } : current)
    queryClient.setQueriesData({ queryKey: ['current-round', mediaId] }, (current: unknown) => {
      if (!current || typeof current !== 'object') return current
      return { ...current, profileVersion: version }
    })
  }
  const savedMovieRound = (saved: CurrentRound) => {
    queryClient.setQueryData(['current-round', mediaId, 'movie'], saved)
    queryClient.setQueryData<RecordState>(['record', mediaId], (current) => current ? {
      ...current,
      status: saved.status,
      rating: saved.rating,
      note: saved.note,
      watchedAt: saved.watchedAt,
      viewingMethod: saved.viewingMethod,
    } : current)
  }
  const organizing = (profileVersion: number, activeRound?: CurrentRound) => (
    <details className="record-organizing" onToggle={(event) => setOrganizingOpen(event.currentTarget.open)}>
      <summary>家庭与整理</summary>
      {organizingOpen ? (
        <div className="record-organizing-content">
          <RecordTagsEditor mediaID={mediaId} version={profileVersion} onVersionChange={updateProfileVersion} />
          <CollectionPicker mediaID={mediaId} />
          <RecordSharingEditor mediaID={mediaId} version={profileVersion} onVersionChange={updateProfileVersion} />
          <HouseholdSharedRecords mediaID={mediaId} members={participants.data ?? []} />
          {activeRound ? (
            <RecordDangerZone
              mediaID={mediaId}
              round={activeRound}
              onRoundChange={(next) => {
                if (next.seasonNumber === null) savedMovieRound(next)
                else {
                  queryClient.setQueryData(['current-round', mediaId, next.seasonNumber], next)
                }
              }}
            />
          ) : null}
        </div>
      ) : null}
    </details>
  )

  return (
    <div className="page media-details-page media-atmosphere-page" style={atmosphere.style}>
      <MediaHero
        media={media.data}
        record={record.data}
        external={external.data}
        linker={<TMDBLinker media={media.data} />}
        onPaletteChange={atmosphere.onPaletteChange}
      />

      {credits.data?.crew && credits.data.crew.length > 0 ? (
        <p className="media-crew-line" aria-label="主创">
          {credits.data.crew.map((member) => `${member.job} ${member.name}`).join(' · ')}
        </p>
      ) : null}
      <CastStrip
        cast={credits.data?.cast ?? []}
        pending={Boolean(tmdbID) && credits.isPending}
        error={credits.isError}
        linked={Boolean(tmdbID)}
        onRetry={() => void credits.refetch()}
      />

      {media.data.mediaType === 'tv' ? (
        <SeasonRecordWorkspace
          mediaId={mediaId}
          tmdbId={tmdbID}
          participants={participants.data ?? []}
          organizing={organizing}
        />
      ) : movieRound.data ? (
        <section className="movie-record-workspace">
          <aside className="personal-record-panel movie-record-panel" aria-label="个人记录">
            <RoundRecordForm
              round={movieRound.data}
              now={new Date()}
              participants={participants.data ?? []}
              onSaved={savedMovieRound}
            />
            {organizing(movieRound.data.profileVersion, movieRound.data)}
          </aside>
          <RewatchSection round={movieRound.data} onRewatched={savedMovieRound} />
        </section>
      ) : null}

      {tmdbID && mediaType ? <MediaStills mediaType={mediaType} tmdbId={tmdbID} /> : null}
    </div>
  )
}

function DetailsSkeleton() {
  return (
    <div className="page details-skeleton" aria-label="正在加载影视详情">
      <div className="skeleton poster-skeleton" />
      <div className="skeleton copy-skeleton" />
    </div>
  )
}
