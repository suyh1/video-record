import { useQueries, useQuery } from '@tanstack/react-query'
import { RefreshCw, Search } from 'lucide-react'
import { type CSSProperties, useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'

import type { MediaSearchResult, TMDBHighlight } from '../../api/types'
import { getCurrentRound, getEpisodeProgress, getTMDBSeason, getTMDBTV } from '../../api/client'
import { sampleMediaAccent } from '../../lib/mediaAccent'
import { findNextEpisode, mergeSeason, regularSeasons, selectActiveSeason } from '../episodes/episodeCatalog'
import { BackdropCarousel } from '../highlights/BackdropCarousel'

export type HomeHeroItem = TMDBHighlight & {
  localItem?: MediaSearchResult
}

export type HomeHeroProps = {
  isError: boolean
  isLoading: boolean
  items: HomeHeroItem[]
  onBackdropStateChange?: (state: HomeHeroBackdropState) => void
  onRetry: () => void
  onSearch?: () => void
}

export type HomeHeroBackdropState = 'empty' | 'loading' | 'ready'

export function HomeHero({
  isError,
  isLoading,
  items,
  onBackdropStateChange,
  onRetry,
  onSearch,
}: HomeHeroProps) {
  const sessionKey = useMemo(
    () => items.map((item) => `${item.mediaType}:${item.id}:${item.backdropURL}`).join('|'),
    [items],
  )
  const [activeItem, setActiveItem] = useState<HomeHeroItem | null>(null)
  const [mediaAccent, setMediaAccent] = useState('var(--primary)')
  const [carouselSettled, setCarouselSettled] = useState(items.length === 0)

  useEffect(() => {
    setActiveItem(null)
    setMediaAccent('var(--primary)')
    setCarouselSettled(items.length === 0)
  }, [items.length, sessionKey])

  const handleActiveItemChange = useCallback((item: TMDBHighlight | null) => {
    setActiveItem(item as HomeHeroItem | null)
    setCarouselSettled(true)
  }, [])
  const handleActiveImageChange = useCallback((image: HTMLImageElement | null) => {
    setMediaAccent(image ? (sampleMediaAccent(image) ?? 'var(--primary)') : 'var(--primary)')
  }, [])

  const empty = !isLoading && (items.length === 0 || (carouselSettled && activeItem === null))
  const loading = activeItem === null && (isLoading || !empty)
  const state: HomeHeroBackdropState = empty ? 'empty' : activeItem ? 'ready' : 'loading'

  useEffect(() => {
    onBackdropStateChange?.(state)
  }, [onBackdropStateChange, state])

  return (
    <section
      aria-label="首页主视觉"
      className={`home-hero ${empty ? 'is-empty' : activeItem ? 'has-backdrop' : 'is-loading'}`}
      data-backdrop-state={state}
      style={{ '--media-accent': mediaAccent } as CSSProperties}
    >
      {items.length ? (
        <BackdropCarousel
          intervalMs={8_000}
          items={items}
          onActiveImageChange={handleActiveImageChange}
          onActiveItemChange={handleActiveItemChange}
          showControls
        />
      ) : null}

      {activeItem ? (
        <div className="home-hero__content">
          <p className="home-hero__meta">{activeItem.year} · {activeItem.mediaType === 'movie' ? '电影' : '剧集'}</p>
          <h1>{activeItem.title}</h1>
          {activeItem.overview ? <p className="home-hero__overview">{activeItem.overview}</p> : null}
          {activeItem.localItem ? (
            <HomeHeroPrivateAction key={activeItem.localItem.id} item={activeItem.localItem} />
          ) : null}
          <div className="home-hero__progress">
            <span aria-label={`第 ${items.indexOf(activeItem) + 1} 张，共 ${items.length} 张`}>
              {items.indexOf(activeItem) + 1} / {items.length}
            </span>
            <ol aria-hidden="true">
              {items.map((item) => (
                <li
                  className={item === activeItem ? 'is-active' : ''}
                  key={`${item.mediaType}:${item.id}`}
                />
              ))}
            </ol>
          </div>
        </div>
      ) : null}

      {empty ? (
        <div className="home-hero__empty">
          <h1>首页</h1>
          {isError ? (
            <div className="home-hero__error" role="alert">
              <p>主视觉暂时无法加载</p>
              <button className="button-secondary" type="button" onClick={onRetry}>
                <RefreshCw aria-hidden="true" size={18} />
                重试主视觉
              </button>
            </div>
          ) : <p>还没有可展示的影视背景</p>}
          {!isError && onSearch ? (
            <button className="button-secondary" type="button" onClick={onSearch}>
              <Search aria-hidden="true" size={18} />
              搜索影视
            </button>
          ) : null}
        </div>
      ) : null}

      {loading ? <div className="home-hero__skeleton skeleton" aria-label="正在加载首页主视觉" /> : null}
    </section>
  )
}

function HomeHeroPrivateAction({ item }: { item: MediaSearchResult }) {
  if (item.mediaType !== 'tv' || item.status !== 'watching' || !item.tmdbId) {
    return <HomeHeroRecordLink item={item} />
  }
  return <WatchingSeriesHeroAction item={item} />
}

function WatchingSeriesHeroAction({ item }: { item: MediaSearchResult }) {
  const tv = useQuery({
    queryKey: ['tmdb-tv', item.tmdbId],
    queryFn: ({ signal }) => getTMDBTV(item.tmdbId ?? 0, signal),
  })
  const seasons = regularSeasons(tv.data?.seasons ?? [])
  const rounds = useQueries({
    queries: seasons.map((season) => ({
      queryKey: ['current-round', item.id, season.seasonNumber],
      queryFn: ({ signal }: { signal: AbortSignal }) => getCurrentRound(item.id, season.seasonNumber, signal),
    })),
  })
  const roundsPending = rounds.some((round) => round.isPending)
  const activeSeason = roundsPending
    ? null
    : selectActiveSeason(seasons, rounds.flatMap((round) => round.data ? [round.data] : []))
  const progress = useQuery({
    queryKey: ['episode-progress', item.id, activeSeason],
    queryFn: ({ signal }) => getEpisodeProgress(item.id, activeSeason ?? 0, signal),
    enabled: activeSeason !== null,
  })
  const season = useQuery({
    queryKey: ['tmdb-season', item.tmdbId, activeSeason],
    queryFn: ({ signal }) => getTMDBSeason(item.tmdbId ?? 0, activeSeason ?? 0, signal),
    enabled: activeSeason !== null,
  })
  const mergedSeason = tv.data && season.data && progress.data
    ? mergeSeason(season.data, tv.data.seasons, progress.data)
    : null
  const nextEpisode = mergedSeason ? findNextEpisode(mergedSeason.episodes) : null
  const catalogUnavailable = tv.isError
    || rounds.some((round) => round.isError)
    || progress.isError
    || season.isError
    || (!tv.isPending && !roundsPending && activeSeason === null)

  if (!nextEpisode || catalogUnavailable) return <HomeHeroRecordLink item={item} />

  const label = episodeLabel(nextEpisode)
  return (
    <div className="home-hero__private-action">
      <p>下一集 · {nextEpisode.name || label}</p>
      <Link className="button-primary home-hero__action" to={`/media/${item.id}`}>继续 {label}</Link>
    </div>
  )
}

function HomeHeroRecordLink({ item }: { item: MediaSearchResult }) {
  return <Link className="button-primary home-hero__action" to={`/media/${item.id}`}>查看记录</Link>
}

function episodeLabel(episode: { seasonNumber: number; episodeNumber: number }) {
  return `S${String(episode.seasonNumber).padStart(2, '0')}E${String(episode.episodeNumber).padStart(2, '0')}`
}
