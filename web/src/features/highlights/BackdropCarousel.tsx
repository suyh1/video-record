import { ChevronLeft, ChevronRight, Pause, Play } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { TMDBHighlight } from '../../api/types'

export type BackdropCarouselProps = {
  items: TMDBHighlight[]
  intervalMs: number
  showControls?: boolean
  onActiveImageChange?: (image: HTMLImageElement | null, item: TMDBHighlight | null) => void
  onActiveItemChange?: (item: TMDBHighlight | null) => void
}

type ImageStatus = 'idle' | 'ready' | 'failed'

type NavigationIntent = {
  direction: 1 | -1
  originIndex: number
}

export function BackdropCarousel(props: BackdropCarouselProps) {
  const sessionKey = useMemo(
    () => props.items.map((item) => `${item.mediaType}:${item.id}:${item.backdropURL}`).join('|'),
    [props.items],
  )

  return <BackdropCarouselSession key={sessionKey} {...props} />
}

function BackdropCarouselSession({
  intervalMs,
  items,
  onActiveImageChange,
  onActiveItemChange,
  showControls = false,
}: BackdropCarouselProps) {
  const [statuses, setStatuses] = useState<ImageStatus[]>(() => items.map(() => 'idle'))
  const [loadingIndex, setLoadingIndex] = useState<number | null>(null)
  const [activeIndex, setActiveIndex] = useState<number | null>(null)
  const [previousIndex, setPreviousIndex] = useState<number | null>(null)
  const [manuallyPaused, setManuallyPaused] = useState(false)
  const [navigationIntent, setNavigationIntent] = useState<NavigationIntent | null>(null)
  const initialCandidatePending = useRef(true)
  const decodedImages = useRef<Array<HTMLImageElement | null>>(items.map(() => null))
  const documentVisible = useDocumentVisibility()
  const reducedMotion = useReducedMotion()
  const loadingURL = loadingIndex === null ? null : items[loadingIndex]?.backdropURL ?? null

  useEffect(() => {
    if (navigationIntent) {
      if (activeIndex !== navigationIntent.originIndex) {
        setNavigationIntent(null)
        return
      }

      for (let offset = 1; offset < items.length; offset += 1) {
        const candidateIndex = (
          navigationIntent.originIndex
          + navigationIntent.direction * offset
          + items.length
        ) % items.length
        const status = statuses[candidateIndex]
        if (status === 'failed') continue
        if (status === 'ready') {
          if (loadingIndex !== null) setLoadingIndex(null)
          setPreviousIndex(activeIndex)
          setActiveIndex(candidateIndex)
          setNavigationIntent(null)
          return
        }
        if (status === 'idle' && loadingIndex !== candidateIndex) {
          setLoadingIndex(candidateIndex)
        }
        return
      }

      setNavigationIntent(null)
      return
    }

    if (loadingIndex !== null) return

    if (activeIndex === null) {
      const readyIndex = statuses.findIndex((status) => status === 'ready')
      if (readyIndex >= 0) {
        setActiveIndex(readyIndex)
        return
      }

      const idleIndex = statuses.findIndex((status) => status === 'idle')
      if (idleIndex >= 0) setLoadingIndex(idleIndex)
      return
    }

    for (let offset = 1; offset < items.length; offset += 1) {
      const candidateIndex = (activeIndex + offset) % items.length
      const status = statuses[candidateIndex]
      if (status === 'ready') return
      if (status === 'idle') {
        setLoadingIndex(candidateIndex)
        return
      }
    }
  }, [activeIndex, items.length, loadingIndex, navigationIntent, statuses])

  useEffect(() => {
    if (loadingIndex === null || loadingURL === null) return

    let current = true
    const image = new Image()
    const isInitialCandidate = initialCandidatePending.current
    if (isInitialCandidate) image.fetchPriority = 'high'
    let settled = false
    const settle = (status: Extract<ImageStatus, 'ready' | 'failed'>) => {
      if (!current || settled) return
      settled = true
      if (isInitialCandidate) initialCandidatePending.current = false
      decodedImages.current[loadingIndex] = status === 'ready' ? image : null
      setStatuses((currentStatuses) => currentStatuses.map((currentStatus, index) => (
        index === loadingIndex ? status : currentStatus
      )))
      setLoadingIndex(null)
    }

    image.onerror = () => settle('failed')
    image.onload = () => {
      if (typeof image.decode !== 'function') settle('ready')
    }
    image.src = loadingURL

    if (typeof image.decode === 'function') {
      image.decode().then(
        () => settle('ready'),
        () => settle('failed'),
      )
    }

    return () => {
      current = false
      image.onerror = null
      image.onload = null
    }
  }, [loadingIndex, loadingURL])

  const readyCount = statuses.filter((status) => status === 'ready').length
  const isEmpty = items.length === 0 || (
    activeIndex === null
    && loadingIndex === null
    && statuses.every((status) => status === 'failed')
  )

  useEffect(() => {
    if (activeIndex !== null) {
      onActiveItemChange?.(items[activeIndex] ?? null)
      onActiveImageChange?.(decodedImages.current[activeIndex] ?? null, items[activeIndex] ?? null)
    } else if (isEmpty) {
      onActiveItemChange?.(null)
      onActiveImageChange?.(null, null)
    }
  }, [activeIndex, isEmpty, items, onActiveImageChange, onActiveItemChange])

  const advance = useCallback((direction: 1 | -1) => {
    if (activeIndex === null) return

    for (let offset = 1; offset < items.length; offset += 1) {
      const candidateIndex = (activeIndex + direction * offset + items.length) % items.length
      if (statuses[candidateIndex] === 'ready') {
        setPreviousIndex(activeIndex)
        setActiveIndex(candidateIndex)
        return
      }
    }
  }, [activeIndex, items.length, statuses])

  useEffect(() => {
    if (
      readyCount < 2
      || intervalMs <= 0
      || manuallyPaused
      || !documentVisible
      || reducedMotion
    ) return

    const timer = window.setTimeout(() => advance(1), intervalMs)
    return () => window.clearTimeout(timer)
  }, [advance, documentVisible, intervalMs, manuallyPaused, readyCount, reducedMotion])

  const navigate = (direction: 1 | -1) => {
    setManuallyPaused(true)
    if (activeIndex !== null) setNavigationIntent({ direction, originIndex: activeIndex })
  }

  const handleLayerError = (failedIndex: number) => {
    setStatuses((currentStatuses) => currentStatuses.map((status, index) => (
      index === failedIndex ? 'failed' : status
    )))
    if (previousIndex === failedIndex) setPreviousIndex(null)
    if (activeIndex !== failedIndex) return

    let nextIndex: number | null = null
    for (let offset = 1; offset < items.length; offset += 1) {
      const candidateIndex = (failedIndex + offset) % items.length
      if (statuses[candidateIndex] === 'ready') {
        nextIndex = candidateIndex
        break
      }
    }
    setPreviousIndex(null)
    setActiveIndex(nextIndex)
  }

  const activeItem = activeIndex === null ? null : items[activeIndex] ?? null
  const previousItem = previousIndex === null ? null : items[previousIndex] ?? null
  const stateClass = isEmpty ? 'is-empty' : activeItem ? 'is-ready' : 'is-loading'
  const controlsEnabled = readyCount >= 2

  return (
    <div className={`backdrop-carousel ${stateClass}`}>
      <div className="backdrop-carousel__layers">
        {previousItem && previousIndex !== activeIndex ? (
          <img
            key={`${previousIndex}:${previousItem.mediaType}:${previousItem.id}:${previousItem.backdropURL}`}
            alt=""
            aria-hidden="true"
            className="backdrop-carousel__image is-previous"
            draggable={false}
            onError={() => handleLayerError(previousIndex!)}
            src={previousItem.backdropURL}
          />
        ) : null}
        {activeItem ? (
          <img
            key={`${activeIndex}:${activeItem.mediaType}:${activeItem.id}:${activeItem.backdropURL}`}
            alt=""
            aria-hidden="true"
            className="backdrop-carousel__image is-active"
            draggable={false}
            onError={() => handleLayerError(activeIndex!)}
            src={activeItem.backdropURL}
          />
        ) : null}
      </div>

      {showControls ? (
        <div className="backdrop-carousel__controls">
          <button
            aria-label="上一张背景"
            className="backdrop-carousel__control"
            disabled={!controlsEnabled}
            onClick={() => navigate(-1)}
            title="上一张背景"
            type="button"
          >
            <ChevronLeft aria-hidden="true" />
          </button>
          <button
            aria-label="下一张背景"
            className="backdrop-carousel__control"
            disabled={!controlsEnabled}
            onClick={() => navigate(1)}
            title="下一张背景"
            type="button"
          >
            <ChevronRight aria-hidden="true" />
          </button>
          <button
            aria-label={manuallyPaused ? '继续轮播' : '暂停轮播'}
            aria-pressed={manuallyPaused}
            className="backdrop-carousel__control"
            disabled={!controlsEnabled || reducedMotion}
            onClick={() => setManuallyPaused((paused) => !paused)}
            title={manuallyPaused ? '继续轮播' : '暂停轮播'}
            type="button"
          >
            {manuallyPaused ? <Play aria-hidden="true" /> : <Pause aria-hidden="true" />}
          </button>
        </div>
      ) : null}
    </div>
  )
}

function useDocumentVisibility() {
  const [visible, setVisible] = useState(() => document.visibilityState !== 'hidden')

  useEffect(() => {
    const handleVisibilityChange = () => setVisible(document.visibilityState !== 'hidden')
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange)
  }, [])

  return visible
}

function useReducedMotion() {
  const [reduced, setReduced] = useState(() => (
    typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false
  ))

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const query = window.matchMedia('(prefers-reduced-motion: reduce)')
    const handleChange = (event: MediaQueryListEvent) => setReduced(event.matches)
    setReduced(query.matches)
    query.addEventListener('change', handleChange)
    return () => query.removeEventListener('change', handleChange)
  }, [])

  return reduced
}
