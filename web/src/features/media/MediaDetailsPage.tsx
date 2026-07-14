import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Star } from 'lucide-react'
import { useParams } from 'react-router-dom'

import { getHouseholdParticipants, getMedia, getRecord, getWatchEvents } from '../../api/client'
import type { MediaSearchResult, RecordState } from '../../api/types'
import { EpisodeProgress } from '../episodes/EpisodeProgress'
import { CollectionPicker } from '../collections/CollectionPicker'
import { QuickRecordForm } from '../records/QuickRecordForm'
import { RecordTagsEditor } from '../records/RecordTagsEditor'
import { RecordSharingEditor } from '../records/RecordSharingEditor'
import { WatchHistory } from '../records/WatchHistory'
import { HouseholdSharedRecords } from '../records/HouseholdSharedRecords'
import { MediaPoster } from './MediaPoster'
import { TMDBLinker } from './TMDBLinker'

export function MediaDetailsPage() {
  const { mediaId = '' } = useParams()
  const queryClient = useQueryClient()
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

  if (media.isPending || record.isPending || events.isPending) return <DetailsSkeleton />
  if (media.isError || record.isError || events.isError) {
    return (
      <div className="page page-error" role="alert">
        <h1>无法打开影视详情</h1>
        <p>记录仍保存在服务器中，请稍后重试。</p>
      </div>
    )
  }

  const posterItem: MediaSearchResult = {
    id: media.data.id,
    source: 'local',
    mediaType: media.data.mediaType,
    title: media.data.title,
    originalTitle: media.data.originalTitle,
    year: media.data.releaseDate.slice(0, 4),
    posterPath: media.data.posterPath,
    status: record.data.status,
  }
  const savedRecord = (nextRecord: RecordState) => {
    queryClient.setQueryData(['record', mediaId], nextRecord)
    void queryClient.invalidateQueries({ queryKey: ['watch-events', mediaId] })
  }

  return (
    <div className="page media-details-page">
      <header className="media-details-header">
        <MediaPoster item={posterItem} />
        <div className="media-title-block">
          <p className="media-type-label">{media.data.mediaType === 'movie' ? '电影' : '剧集'}</p>
          <h1>{media.data.title}</h1>
          <p className="media-original-title">{media.data.originalTitle}</p>
          <p className="media-release-year">{media.data.releaseDate.slice(0, 4)}</p>
          <TMDBLinker media={media.data} />
        </div>
      </header>

      <section className="details-section personal-record" aria-labelledby="personal-record-heading">
        <div className="details-section-heading">
          <div>
            <h2 id="personal-record-heading">个人记录</h2>
            <p>只有你能看到评分和私人笔记</p>
          </div>
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
        <RecordTagsEditor
          mediaID={mediaId}
          version={record.data.version}
          onVersionChange={(version) => queryClient.setQueryData<RecordState>(
            ['record', mediaId],
            (current) => current ? { ...current, version } : current,
          )}
        />
        <RecordSharingEditor
          mediaID={mediaId}
          version={record.data.version}
          onVersionChange={(version) => queryClient.setQueryData<RecordState>(
            ['record', mediaId],
            (current) => current ? { ...current, version } : current,
          )}
        />
        <CollectionPicker mediaID={mediaId} />
      </section>

      <HouseholdSharedRecords mediaID={mediaId} members={participants.data ?? []} />

      <section className="details-section" aria-labelledby="history-heading">
        <div className="details-section-heading">
          <div>
            <h2 id="history-heading">观看历史</h2>
            <p>{events.data.length} 次记录</p>
          </div>
        </div>
        <WatchHistory mediaID={mediaId} events={events.data} />
      </section>

      {media.data.mediaType === 'tv' ? <EpisodeProgress mediaId={mediaId} /> : null}

      <section className="details-section" aria-labelledby="overview-heading">
        <h2 id="overview-heading">简介</h2>
        <p className="media-overview">{media.data.overview || '暂无简介'}</p>
      </section>
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
