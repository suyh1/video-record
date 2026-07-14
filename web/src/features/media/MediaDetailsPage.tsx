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
import { EpisodeProgress } from '../episodes/EpisodeProgress'
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
    enabled: Boolean(mediaId),
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

  if (media.isPending || record.isPending || events.isPending) return <DetailsSkeleton />
  if (media.isError || record.isError || events.isError) {
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

      <div className="media-details-layout">
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

          <details className="record-organizing" onToggle={(event) => setOrganizingOpen(event.currentTarget.open)}>
            <summary>家庭与整理</summary>
            {organizingOpen ? (
              <div className="record-organizing-content">
                <RecordTagsEditor mediaID={mediaId} version={record.data.version} onVersionChange={updateVersion} />
                <CollectionPicker mediaID={mediaId} />
                <RecordSharingEditor mediaID={mediaId} version={record.data.version} onVersionChange={updateVersion} />
                <HouseholdSharedRecords mediaID={mediaId} members={participants.data ?? []} />
              </div>
            ) : null}
          </details>
        </aside>

        <div className="media-details-primary">
          {media.data.mediaType === 'tv' ? <EpisodeProgress mediaId={mediaId} tmdbId={tmdbID} /> : null}
          <section className="details-section" aria-labelledby="history-heading">
            <div className="details-section-heading">
              <div><h2 id="history-heading">观看历史</h2><p>{events.data.length} 次记录</p></div>
            </div>
            <WatchHistory mediaID={mediaId} events={events.data} />
          </section>
        </div>
      </div>
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
