import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Star } from 'lucide-react'
import { useState } from 'react'
import { useParams } from 'react-router-dom'

import {
  getHouseholdParticipants,
  getMedia,
  getRecord,
  getTMDBCredits,
  getTMDBMovie,
  getTMDBTV,
  getWatchEvents,
} from '../../api/client'
import type { RecordState, TMDBMovieDetails, TMDBTVDetails } from '../../api/types'
import { CollectionPicker } from '../collections/CollectionPicker'
import { SeasonRecordWorkspace } from '../episodes/SeasonRecordWorkspace'
import { HouseholdSharedRecords } from '../records/HouseholdSharedRecords'
import { QuickRecordForm } from '../records/QuickRecordForm'
import { RecordSharingEditor } from '../records/RecordSharingEditor'
import { RecordTagsEditor } from '../records/RecordTagsEditor'
import { WatchHistory } from '../records/WatchHistory'
import { CastStrip } from './CastStrip'
import { MediaHero } from './MediaHero'
import { TMDBLinker } from './TMDBLinker'

export function MediaDetailsPage() {
  const { mediaId = '' } = useParams()
  const queryClient = useQueryClient()
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
  const events = useQuery({
    queryKey: ['watch-events', mediaId],
    queryFn: ({ signal }) => getWatchEvents(mediaId, signal),
    enabled: Boolean(mediaId && media.data?.mediaType === 'movie'),
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

  const eventsPending = media.data?.mediaType === 'movie' && events.isPending
  const eventsError = media.data?.mediaType === 'movie' && events.isError
  if (media.isPending || record.isPending || eventsPending) return <DetailsSkeleton />
  if (media.isError || record.isError || eventsError) {
    return (
      <div className="page page-error" role="alert">
        <h1>无法打开影视详情</h1>
        <p>记录仍保存在服务器中，请稍后重试。</p>
      </div>
    )
  }

  const savedRecord = (nextRecord: RecordState) => {
    queryClient.setQueryData(['record', mediaId], nextRecord)
    void queryClient.invalidateQueries({ queryKey: ['watch-events', mediaId] })
  }
  const updateVersion = (version: number) => queryClient.setQueryData<RecordState>(
    ['record', mediaId],
    (current) => current ? { ...current, version } : current,
  )
  const updateProfileVersion = (version: number) => {
    updateVersion(version)
    queryClient.setQueriesData({ queryKey: ['current-round', mediaId] }, (current: unknown) => {
      if (!current || typeof current !== 'object') return current
      return { ...current, profileVersion: version }
    })
  }
  const organizing = (profileVersion: number) => (
    <details className="record-organizing" onToggle={(event) => setOrganizingOpen(event.currentTarget.open)}>
      <summary>家庭与整理</summary>
      {organizingOpen ? (
        <div className="record-organizing-content">
          <RecordTagsEditor mediaID={mediaId} version={profileVersion} onVersionChange={updateProfileVersion} />
          <CollectionPicker mediaID={mediaId} />
          <RecordSharingEditor mediaID={mediaId} version={profileVersion} onVersionChange={updateProfileVersion} />
          <HouseholdSharedRecords mediaID={mediaId} members={participants.data ?? []} />
        </div>
      ) : null}
    </details>
  )
  const movieEvents = events.data ?? []

  return (
    <div className="page media-details-page">
      <MediaHero
        media={media.data}
        record={record.data}
        external={external.data}
        linker={<TMDBLinker media={media.data} />}
      />

      <CastStrip
        cast={credits.data ?? []}
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
      ) : <div className="media-details-layout">
        <aside className="personal-record-panel" aria-labelledby="personal-record-heading">
          <div className="details-section-heading">
            <div><h2 id="personal-record-heading">个人记录</h2><p>评分和私人笔记仅自己可见</p></div>
            {record.data.rating !== null ? (
              <span className="personal-rating"><Star aria-hidden="true" size={16} />{record.data.rating.toFixed(1)} / 10</span>
            ) : null}
          </div>
          {record.data.note ? <p className="personal-note">{record.data.note}</p> : null}
          <QuickRecordForm
            record={record.data}
            now={new Date()}
            participants={participants.data ?? []}
            onSaved={savedRecord}
            onRewatched={() => void queryClient.invalidateQueries({ queryKey: ['watch-events', mediaId] })}
          />

          {organizing(record.data.version)}
        </aside>

        <div className="media-details-primary">
          <section className="details-section" aria-labelledby="history-heading">
            <div className="details-section-heading">
              <div><h2 id="history-heading">观看历史</h2><p>{movieEvents.length} 次记录</p></div>
            </div>
            <WatchHistory mediaID={mediaId} events={movieEvents} />
          </section>
        </div>
      </div>}
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
