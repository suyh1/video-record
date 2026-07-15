import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bookmark, Check, ChevronRight, CircleStop, Clapperboard, LoaderCircle, Play, RefreshCw, RotateCcw, Search, type LucideIcon } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'

import {
  getEpisodeProgress,
  getCurrentRound,
  getLibrary,
  getTMDBHighlights,
  getTMDBMovie,
  getTMDBSeason,
  getTMDBTV,
  updateEpisodeProgress,
  type UpdateEpisodeProgressPayload,
} from '../../api/client'
import type { EpisodeProgressItem, EpisodeReference, MediaSearchResult, RecordStatus } from '../../api/types'
import { signedTMDBProxyImageURL } from '../../lib/mediaImage'
import { findNextEpisode, mergeSeason, regularSeasons, selectActiveSeason } from '../episodes/episodeCatalog'
import { MediaPoster } from '../media/MediaPoster'
import { HomeHero, type HomeHeroBackdropState, type HomeHeroItem } from './HomeHero'

const statusDetails = {
  none: { label: '未记录', icon: Clapperboard },
  wishlist: { label: '想看', icon: Bookmark },
  watching: { label: '在看', icon: Play },
  completed: { label: '看过', icon: Check },
  dropped: { label: '弃看', icon: CircleStop },
} satisfies Record<RecordStatus, { label: string; icon: typeof Clapperboard }>

export function HomePage({ onHeroBackdropStateChange, onSearch }: {
  onHeroBackdropStateChange?: (state: HomeHeroBackdropState) => void
  onSearch?: () => void
}) {
  const continuing = useQuery({
    queryKey: ['library', 'watching'],
    queryFn: ({ signal }) => getLibrary('watching', signal),
  })
  const recent = useQuery({
    queryKey: ['library', 'all'],
    queryFn: ({ signal }) => getLibrary('all', signal),
  })
  const privateCandidates = useMemo(
    () => collectPrivateHeroCandidates(continuing.data?.items ?? [], recent.data?.items ?? []),
    [continuing.data?.items, recent.data?.items],
  )
  const privateDetails = useQueries({
    queries: privateCandidates.map((item) => ({
      queryKey: [`tmdb-${item.mediaType}`, item.tmdbId],
      queryFn: ({ signal }: { signal: AbortSignal }) => item.mediaType === 'movie'
        ? getTMDBMovie(item.tmdbId ?? 0, signal)
        : getTMDBTV(item.tmdbId ?? 0, signal),
    })),
  })
  const privateDetailsPending = privateDetails.some((detail) => detail.isPending)
  const privateHeroItems = useMemo(() => privateCandidates.flatMap((item, index) => {
    const detail = privateDetails[index]?.data
    const backdropURL = signedTMDBProxyImageURL(detail?.backdropPath)
    if (!detail || !backdropURL || !item.tmdbId) return []
    return [{
      id: item.tmdbId,
      mediaType: item.mediaType,
      title: item.title,
      originalTitle: item.originalTitle,
      year: item.year,
      overview: detail.overview,
      backdropURL,
      localItem: item,
    } satisfies HomeHeroItem]
  }), [privateCandidates, privateDetails])
  const librariesPending = continuing.isPending || recent.isPending
  const needsHighlights = !librariesPending && !privateDetailsPending && privateHeroItems.length < 6
  const highlights = useQuery({
    queryKey: ['tmdb-highlights'],
    queryFn: ({ signal }) => getTMDBHighlights(signal),
    enabled: needsHighlights,
  })
  const heroItems = useMemo(() => {
    const seen = new Set(privateHeroItems.map((item) => `${item.mediaType}:${item.id}`))
    const popular = (highlights.data ?? []).flatMap((item) => {
      const backdropURL = signedTMDBProxyImageURL(item.backdropURL)
      const key = `${item.mediaType}:${item.id}`
      if (!backdropURL || seen.has(key)) return []
      seen.add(key)
      return [{ ...item, backdropURL }]
    })
    return [...privateHeroItems, ...popular].slice(0, 6)
  }, [highlights.data, privateHeroItems])
  const heroLoading = librariesPending
    || privateDetailsPending
    || (needsHighlights && highlights.isPending)
  const heroError = !heroLoading
    && heroItems.length === 0
    && (privateDetails.some((detail) => detail.isError) || highlights.isError)

  const retryHero = () => {
    privateDetails.forEach((detail) => {
      if (detail.isError) void detail.refetch()
    })
    if (highlights.isError) void highlights.refetch()
  }
  const continuingItems = continuing.data?.items.filter((item) => item.mediaType === 'tv') ?? []

  return (
    <div className="page home-page">
      <HomeHero
        isError={heroError}
        isLoading={heroLoading}
        items={heroItems}
        {...(onHeroBackdropStateChange ? { onBackdropStateChange: onHeroBackdropStateChange } : {})}
        onRetry={retryHero}
        {...(onSearch ? { onSearch } : {})}
      />

      <div className="home-content">
      <section className="content-section" aria-labelledby="continue-heading">
        <div className="section-heading">
          <div>
            <h2 id="continue-heading">继续观看</h2>
            <p>{continuingItems.length} 部剧集</p>
          </div>
        </div>
        {continuing.isPending ? <HomePosterSkeleton /> : null}
        {continuing.isError ? (
          <HomeSectionError label="无法读取继续观看" retryLabel="重试继续观看" onRetry={() => { void continuing.refetch() }} />
        ) : null}
        {!continuing.isError && continuingItems.length ? (
          <div className="home-poster-strip">
            {continuingItems.slice(0, 8).map((item) => <HomeContinueItem key={item.id} item={item} />)}
          </div>
        ) : null}
        {!continuing.isPending && !continuing.isError && continuingItems.length === 0 ? (
          <HomeEmpty
            icon={Clapperboard}
            message="还没有正在观看的剧集"
            actionLabel="搜索剧集"
            {...(onSearch ? { onSearch } : {})}
          />
        ) : null}
      </section>

      <section className="content-section" aria-labelledby="recent-heading">
        <div className="section-heading">
          <div>
            <h2 id="recent-heading">最近记录</h2>
            <p>按最近更新时间排列</p>
          </div>
        </div>
        {recent.isPending ? <HomeRecentSkeleton /> : null}
        {recent.isError ? (
          <HomeSectionError label="无法读取最近记录" retryLabel="重试最近记录" onRetry={() => { void recent.refetch() }} />
        ) : null}
        {!recent.isError && recent.data?.items.length ? (
          <div className="home-recent-layout">
            <RecentFeatured item={recent.data.items[0]!} />
            {recent.data.items.length > 1 ? (
              <ul className="home-recent-list" aria-label="更多最近记录">
                {recent.data.items.slice(1, 8).map((item) => <RecentRecord key={item.id} item={item} />)}
              </ul>
            ) : null}
          </div>
        ) : null}
        {!recent.isError && recent.data?.items.length === 0 ? (
          <HomeEmpty
            icon={Search}
            message="第一条观影记录会显示在这里"
            actionLabel="搜索影视"
            {...(onSearch ? { onSearch } : {})}
          />
        ) : null}
      </section>
      </div>
    </div>
  )
}

function HomeSectionError({ label, onRetry, retryLabel }: {
  label: string
  onRetry: () => void
  retryLabel: string
}) {
  return (
    <div className="home-error" role="alert">
      <span>{label}</span>
      <button type="button" onClick={onRetry}>
        <RefreshCw aria-hidden="true" size={16} />
        {retryLabel}
      </button>
    </div>
  )
}

function collectPrivateHeroCandidates(watching: MediaSearchResult[], recent: MediaSearchResult[]) {
  const candidates: MediaSearchResult[] = []
  const seen = new Set<string>()
  for (const item of [...watching, ...recent]) {
    if (!item.tmdbId || item.tmdbId <= 0) continue
    const key = `${item.mediaType}:${item.tmdbId}`
    if (seen.has(key)) continue
    seen.add(key)
    candidates.push(item)
    if (candidates.length === 6) break
  }
  return candidates
}

function HomeContinueItem({ item }: { item: MediaSearchResult }) {
  const queryClient = useQueryClient()
  const [savedAdvance, setSavedAdvance] = useState<SavedAdvance | null>(null)
  const mounted = useRef(true)
  const undoWindow = useRef<HomeUndoWindow>({
    active: false,
    expired: false,
    undoInFlight: false,
    watchingSynchronized: true,
  })
  const synchronizeWatching = useCallback(() => {
    if (undoWindow.current.watchingSynchronized) return
    undoWindow.current.watchingSynchronized = true
    void queryClient.invalidateQueries({
      exact: true,
      queryKey: ['library', 'watching'],
      refetchType: 'all',
    })
  }, [queryClient])
  const linked = Boolean(item.tmdbId)
  const tv = useQuery({
    queryKey: ['tmdb-tv', item.tmdbId],
    queryFn: ({ signal }) => getTMDBTV(item.tmdbId ?? 0, signal),
    enabled: linked,
  })
  const seasons = regularSeasons(tv.data?.seasons ?? [])
  const rounds = useQueries({
    queries: seasons.map((season) => ({
      queryKey: ['current-round', item.id, season.seasonNumber],
      queryFn: ({ signal }: { signal: AbortSignal }) => getCurrentRound(item.id, season.seasonNumber, signal),
    })),
  })
  const roundsPending = rounds.some((round) => round.isPending)
  const activeSeason = savedAdvance?.episode.seasonNumber
    ?? (roundsPending ? null : selectActiveSeason(seasons, rounds.flatMap((round) => round.data ? [round.data] : [])))
  const progress = useQuery({
    queryKey: ['episode-progress', item.id, activeSeason],
    queryFn: ({ signal }) => getEpisodeProgress(item.id, activeSeason ?? 0, signal),
    enabled: linked && activeSeason !== null,
  })
  const season = useQuery({
    queryKey: ['tmdb-season', item.tmdbId, activeSeason],
    queryFn: ({ signal }) => getTMDBSeason(item.tmdbId ?? 0, activeSeason ?? 0, signal),
    enabled: linked && activeSeason !== null,
  })
  const mutation = useMutation({
    mutationFn: ({ seasonNumber, payload }: HomeProgressMutation) => updateEpisodeProgress(item.id, seasonNumber, payload),
    onSuccess: (nextProgress, variables) => {
      queryClient.setQueryData(['episode-progress', item.id, variables.seasonNumber], nextProgress)
      if (variables.payload.action === 'next') {
        const episode = variables.payload.episodeRefs?.[0]
        if (episode) {
          undoWindow.current = {
            active: mounted.current,
            expired: false,
            undoInFlight: false,
            watchingSynchronized: false,
          }
          if (mounted.current) {
            setSavedAdvance({ episode })
          } else {
            synchronizeWatching()
          }
        }
      }
      void queryClient.invalidateQueries({ exact: true, queryKey: ['library', 'all'] })
    },
    onSettled: (_data, _error, variables) => {
      if (variables.payload.action !== 'undo') return
      undoWindow.current.undoInFlight = false
      synchronizeWatching()
      undoWindow.current.active = false
      if (mounted.current) setSavedAdvance(null)
    },
  })
  useEffect(() => {
    if (!savedAdvance) return
    const timeout = window.setTimeout(() => {
      undoWindow.current.expired = true
      if (undoWindow.current.undoInFlight) return
      synchronizeWatching()
      undoWindow.current.active = false
      setSavedAdvance(null)
    }, 10_000)
    return () => window.clearTimeout(timeout)
  }, [savedAdvance, synchronizeWatching])
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      if (undoWindow.current.active && !undoWindow.current.undoInFlight) synchronizeWatching()
    }
  }, [synchronizeWatching])

  const mergedSeason = tv.data && season.data && progress.data
    ? mergeSeason(season.data, tv.data.seasons, progress.data)
    : null
  const nextEpisode = mergedSeason ? findNextEpisode(mergedSeason.episodes) : null
  const totalEpisodes = mergedSeason?.episodes.length ?? 0
  const progressPending = activeSeason !== null && progress.isPending
  const progressError = activeSeason !== null && progress.isError
  const catalogPending = linked && (tv.isPending || roundsPending || (activeSeason !== null && season.isPending))
  const catalogUnavailable = linked && !catalogPending && (
    tv.isError
    || !tv.data
    || activeSeason === null
    || rounds.some((round) => round.isError)
    || season.isError
    || !season.data
    || season.data.episodes.length === 0
  )
  const complete = linked
    && !catalogUnavailable
    && Boolean(mergedSeason?.episodes.length)
    && nextEpisode === null
  const advance = () => {
    if (!progress.data || activeSeason === null || !nextEpisode) return
    mutation.mutate({
      seasonNumber: activeSeason,
      payload: {
        action: 'next',
        expectedVersion: progress.data.version,
        watchedAt: new Date().toISOString(),
        episodeRefs: [toEpisodeReference(nextEpisode)],
        totalEpisodes,
      },
    })
  }
  const undo = () => {
    if (!progress.data || !savedAdvance || activeSeason === null) return
    undoWindow.current.undoInFlight = true
    mutation.mutate({
      seasonNumber: activeSeason,
      payload: {
        action: 'undo',
        expectedVersion: progress.data.version,
        episodeRefs: [savedAdvance.episode],
        totalEpisodes,
      },
    })
  }
  const savedEpisode = savedAdvance?.episode ?? null

  return (
    <article className="home-continue-item">
      <Link className="poster-link" to={`/media/${item.id}`}><MediaPoster item={item} /></Link>
      {progress.data && activeSeason !== null && mergedSeason ? (
        <div className="home-continue-progress">
          <p>
            <span>第 {activeSeason} 季 · {progress.data.watchedEpisodes}/{totalEpisodes} 集</span>
            {nextEpisode ? <span>下一集 · {nextEpisode.name || episodeLabel(nextEpisode)}</span> : null}
          </p>
          <progress
            aria-label={`${item.title} 第 ${activeSeason} 季进度`}
            max={Math.max(totalEpisodes, 1)}
            value={Math.min(progress.data.watchedEpisodes, totalEpisodes)}
          />
        </div>
      ) : null}
      <div className="home-continue-action">
        {progressPending || catalogPending ? <div className="skeleton home-continue-action-skeleton" aria-label={`正在加载 ${item.title} 的剧集进度`} /> : null}
        {progressError ? <span role="alert">进度暂不可用</span> : null}
        {!progressPending && !catalogPending && !progressError && savedEpisode ? (
          <button type="button" disabled={mutation.isPending} aria-label={`撤销 ${item.title} ${episodeLabel(savedEpisode)}`} onClick={undo}>
            {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <RotateCcw aria-hidden="true" size={16} />}
            撤销 {episodeLabel(savedEpisode)}
          </button>
        ) : null}
        {!progressPending && !catalogPending && !progressError && !catalogUnavailable && !savedEpisode && nextEpisode ? (
          <button type="button" disabled={mutation.isPending} aria-label={`推进 ${item.title} 下一集 ${episodeLabel(nextEpisode)}`} onClick={advance}>
            {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <ChevronRight aria-hidden="true" size={16} />}
            下一集 {episodeLabel(nextEpisode)}
          </button>
        ) : null}
        {!progressPending && !catalogPending && !progressError && catalogUnavailable && !savedEpisode ? (
          <Link className="text-link" to={`/media/${item.id}`}>打开详情继续记录</Link>
        ) : null}
        {!progressPending && !catalogPending && !progressError && !catalogUnavailable && !savedEpisode && complete ? (
          <span><Check aria-hidden="true" size={15} />已全部看完</span>
        ) : null}
        {mutation.isError ? <span role="alert">进度保存失败</span> : null}
        {savedEpisode ? <span className="sr-only" role="status">已推进至 {episodeLabel(savedEpisode)}</span> : null}
      </div>
    </article>
  )
}

function RecentFeatured({ item }: { item: MediaSearchResult }) {
  return (
    <Link
      aria-label={`查看 ${item.title} 记录`}
      className="home-recent-featured"
      to={`/media/${item.id}`}
    >
      <MediaPoster item={item} />
      <ChevronRight aria-hidden="true" size={20} />
    </Link>
  )
}

function RecentRecord({ item }: { item: MediaSearchResult }) {
  const status = statusDetails[item.status]
  const StatusIcon = status.icon
  return (
    <li>
      <Link to={`/media/${item.id}`}>
        <span className="home-recent-mark" aria-hidden="true" />
        <span className="home-recent-copy">
          <strong>{item.title}</strong>
          <span>{item.originalTitle || (item.mediaType === 'movie' ? '电影' : '剧集')}</span>
        </span>
        <span className="home-recent-meta">
          <span>{item.mediaType === 'movie' ? '电影' : '剧集'}{item.year ? ` · ${item.year}` : ''}</span>
          <span className={`record-status ${item.status}`}><StatusIcon aria-hidden="true" size={14} />{status.label}</span>
        </span>
      </Link>
    </li>
  )
}

function HomeEmpty({ icon: Icon, message, actionLabel, onSearch }: {
  icon: LucideIcon
  message: string
  actionLabel: string
  onSearch?: () => void
}) {
  return (
    <div className="empty-state">
      <Icon aria-hidden="true" size={24} strokeWidth={1.6} />
      <p>{message}</p>
      {onSearch ? <button className="text-link home-empty-action" type="button" onClick={onSearch}>{actionLabel}</button> : (
        <Link className="text-link" to="/library">前往影库</Link>
      )}
    </div>
  )
}

function HomePosterSkeleton() {
  return (
    <div className="home-poster-strip" aria-label="正在加载继续观看">
      {Array.from({ length: 4 }, (_, index) => <div key={index} className="skeleton home-poster-skeleton" />)}
    </div>
  )
}

function HomeRecentSkeleton() {
  return (
    <div className="home-recent-skeleton" aria-label="正在加载最近记录">
      {Array.from({ length: 3 }, (_, index) => <div key={index} className="skeleton" />)}
    </div>
  )
}

type SavedAdvance = { episode: EpisodeReference }

type HomeUndoWindow = {
  active: boolean
  expired: boolean
  undoInFlight: boolean
  watchingSynchronized: boolean
}

type HomeProgressMutation = {
  seasonNumber: number
  payload: UpdateEpisodeProgressPayload
}

function toEpisodeReference(episode: Pick<EpisodeReference, 'seasonNumber' | 'episodeNumber' | 'absoluteNumber'> & { id: number }) {
  return {
    sourceId: String(episode.id),
    seasonNumber: episode.seasonNumber,
    episodeNumber: episode.episodeNumber,
    absoluteNumber: episode.absoluteNumber,
  }
}

function episodeLabel(episode: Pick<EpisodeProgressItem, 'seasonNumber' | 'episodeNumber'>) {
  return `S${String(episode.seasonNumber).padStart(2, '0')}E${String(episode.episodeNumber).padStart(2, '0')}`
}
