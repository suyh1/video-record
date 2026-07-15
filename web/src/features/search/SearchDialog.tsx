import * as Dialog from '@radix-ui/react-dialog'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Check, Plus, Search, Trash2, X } from 'lucide-react'
import { type KeyboardEvent, useEffect, useId, useMemo, useRef, useState } from 'react'

import { createCustomMedia, searchLocalMedia, searchTMDB } from '../../api/client'
import type { MediaSearchResult, MediaType } from '../../api/types'
import { MediaPoster } from '../media/MediaPoster'

type SearchDialogProps = {
  open: boolean
  onClose: () => void
  onSelect: (item: MediaSearchResult) => void | Promise<void>
}

const recentSearchesKey = 'video-record.recent-searches'
const recentSearchLimit = 5

type CustomSubmission = {
  epoch: number
  mediaType: MediaType
  query: string
  year: string
}

export function SearchDialog({ open, onClose, onSelect }: SearchDialogProps) {
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [selectingID, setSelectingID] = useState<string | null>(null)
  const [selectionError, setSelectionError] = useState(false)
  const [customOpen, setCustomOpen] = useState(false)
  const [customType, setCustomType] = useState<MediaType>('movie')
  const [customYear, setCustomYear] = useState('')
  const [recentSearches, setRecentSearches] = useState(readRecentSearches)
  const inputRef = useRef<HTMLInputElement>(null)
  const resultsRef = useRef<HTMLDivElement>(null)
  const selectionEpoch = useRef(0)
  const wasOpen = useRef(open)
  const localHeadingID = useId()
  const remoteHeadingID = useId()
  const recentHeadingID = useId()

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedQuery(query.trim()), 300)
    return () => window.clearTimeout(timeout)
  }, [query])

  useEffect(() => {
    if (wasOpen.current && !open) {
      selectionEpoch.current += 1
      setSelectingID(null)
      setSelectionError(false)
      setCustomOpen(false)
    }
    wasOpen.current = open
  }, [open])

  useEffect(() => () => {
    selectionEpoch.current += 1
  }, [])

  const normalizedQuery = query.trim()
  const hasQuery = normalizedQuery.length > 0
  const querySettled = hasQuery && normalizedQuery === debouncedQuery
  const enabled = open && querySettled
  const local = useQuery({
    queryKey: ['media-search', 'local', debouncedQuery],
    queryFn: ({ signal }) => searchLocalMedia(debouncedQuery, signal),
    enabled,
  })
  const remote = useQuery({
    queryKey: ['media-search', 'tmdb', debouncedQuery],
    queryFn: ({ signal }) => searchTMDB(debouncedQuery, signal),
    enabled,
  })
  const localResults = local.data ?? []
  const remoteResults = useMemo(() => deduplicateRemote(localResults, remote.data ?? []), [localResults, remote.data])
  const canCreateCustom = enabled && local.isSuccess && remote.isSuccess
    && localResults.length === 0 && remoteResults.length === 0

  const rememberQuery = (value: string) => {
    const normalized = value.trim()
    if (!normalized) return
    setRecentSearches((current) => {
      const folded = normalized.toLocaleLowerCase()
      const next = [
        normalized,
        ...current.filter((item) => item.toLocaleLowerCase() !== folded),
      ].slice(0, recentSearchLimit)
      writeRecentSearches(next)
      return next
    })
  }

  const customMutation = useMutation({
    mutationFn: (submission: CustomSubmission) => createCustomMedia({
      title: submission.query,
      mediaType: submission.mediaType,
      year: submission.year,
      overview: '',
    }),
    onSuccess: async (media, submission) => {
      if (submission.epoch !== selectionEpoch.current) return
      await onSelect({
        id: media.id,
        source: 'local',
        mediaType: media.mediaType,
        title: media.title,
        originalTitle: media.originalTitle,
        year: media.releaseDate.slice(0, 4),
        posterPath: media.posterPath,
        status: 'none',
      })
      if (submission.epoch === selectionEpoch.current) rememberQuery(submission.query)
    },
  })
  const currentCustomPending = customMutation.isPending
    && customMutation.variables?.epoch === selectionEpoch.current
  const currentCustomError = customMutation.isError
    && customMutation.variables?.epoch === selectionEpoch.current

  const selectResult = async (item: MediaSearchResult, settledQuery: string) => {
    if (selectingID !== null) return
    const epoch = selectionEpoch.current
    setSelectingID(resultKey(item))
    setSelectionError(false)
    try {
      await onSelect(item)
      if (epoch === selectionEpoch.current) rememberQuery(settledQuery)
    } catch {
      if (epoch === selectionEpoch.current) setSelectionError(true)
    } finally {
      if (epoch === selectionEpoch.current) setSelectingID(null)
    }
  }

  const closeSearch = () => {
    selectionEpoch.current += 1
    setSelectingID(null)
    setSelectionError(false)
    setCustomOpen(false)
    onClose()
  }

  const focusResult = (index: number) => {
    resultButtons(resultsRef.current)[index]?.focus()
  }

  const moveResultFocus = (event: KeyboardEvent<HTMLButtonElement>, direction: -1 | 1) => {
    const buttons = resultButtons(resultsRef.current)
    const currentIndex = buttons.indexOf(event.currentTarget)
    if (currentIndex < 0 || buttons.length === 0) return
    event.preventDefault()
    buttons[(currentIndex + direction + buttons.length) % buttons.length]?.focus()
  }

  return (
    <Dialog.Root open={open} onOpenChange={(nextOpen) => !nextOpen && closeSearch()}>
      <Dialog.Portal>
        <Dialog.Overlay className="search-overlay" />
        <Dialog.Content className="search-dialog" onOpenAutoFocus={(event) => {
          event.preventDefault()
          inputRef.current?.focus()
        }}>
          <div className="search-dialog-heading">
            <Dialog.Title>搜索影视</Dialog.Title>
            <Dialog.Close className="icon-button" aria-label="关闭搜索" title="关闭搜索">
              <X aria-hidden="true" size={20} />
            </Dialog.Close>
          </div>
          <Dialog.Description className="search-dialog-description">
            搜索本地影库或 TMDB 影视条目
          </Dialog.Description>
          <div className="search-dialog-input">
            <Search aria-hidden="true" size={18} />
            <input
              ref={inputRef}
              type="search"
              aria-label="搜索影视"
              placeholder="输入电影或剧集名称"
              value={query}
              onChange={(event) => {
                setQuery(event.target.value)
                setSelectionError(false)
                setCustomOpen(false)
                customMutation.reset()
              }}
              onKeyDown={(event) => {
                if (event.key !== 'ArrowDown') return
                const buttons = resultButtons(resultsRef.current)
                if (buttons.length === 0) return
                event.preventDefault()
                focusResult(0)
              }}
            />
          </div>
          <div ref={resultsRef} className="search-results" aria-live="polite">
            {!hasQuery && recentSearches.length > 0 ? (
              <section className="recent-searches" aria-labelledby={recentHeadingID}>
                <div className="search-section-heading">
                  <h3 id={recentHeadingID}>最近搜索</h3>
                  <button
                    className="icon-button"
                    type="button"
                    aria-label="清除最近搜索"
                    title="清除最近搜索"
                    onClick={() => {
                      clearRecentSearches()
                      setRecentSearches([])
                    }}
                  >
                    <Trash2 aria-hidden="true" size={17} />
                  </button>
                </div>
                <div className="recent-search-list">
                  {recentSearches.map((item) => (
                    <button
                      key={item.toLocaleLowerCase()}
                      type="button"
                      onClick={() => {
                        setQuery(item)
                        inputRef.current?.focus()
                      }}
                    >
                      {item}
                    </button>
                  ))}
                </div>
              </section>
            ) : null}
            {hasQuery && !querySettled ? <SearchSkeleton /> : null}
            {enabled ? (
              <>
                <SearchSection
                  heading="本地影库"
                  headingID={localHeadingID}
                  items={localResults}
                  pending={local.isPending}
                  error={local.isError}
                  emptyLabel="本地影库没有匹配结果"
                  selectingID={selectingID}
                  settledQuery={debouncedQuery}
                  onSelect={selectResult}
                  onMoveFocus={moveResultFocus}
                />
                <SearchSection
                  heading="TMDB"
                  headingID={remoteHeadingID}
                  items={remoteResults}
                  pending={remote.isPending}
                  error={remote.isError}
                  emptyLabel="TMDB 没有匹配结果"
                  selectingID={selectingID}
                  settledQuery={debouncedQuery}
                  onSelect={selectResult}
                  onMoveFocus={moveResultFocus}
                />
              </>
            ) : null}
            {canCreateCustom ? (
              <div className="custom-search-empty">
                <p className="search-guidance">没有找到匹配的电影或剧集</p>
                {!customOpen ? (
                  <button type="button" onClick={() => setCustomOpen(true)}><Plus aria-hidden="true" size={16} />创建自定义条目</button>
                ) : (
                  <form className="custom-media-form" onSubmit={(event) => {
                    event.preventDefault()
                    customMutation.mutate({
                      epoch: selectionEpoch.current,
                      mediaType: customType,
                      query: debouncedQuery,
                      year: customYear.trim(),
                    })
                  }}>
                    <fieldset>
                      <legend>媒体类型</legend>
                      <div className="custom-media-types" role="radiogroup" aria-label="媒体类型">
                        <button type="button" role="radio" aria-checked={customType === 'movie'} onClick={() => setCustomType('movie')}>电影</button>
                        <button type="button" role="radio" aria-checked={customType === 'tv'} onClick={() => setCustomType('tv')}>剧集</button>
                      </div>
                    </fieldset>
                    <label><span>年份（可选）</span><input aria-label="年份（可选）" inputMode="numeric" pattern="[0-9]{4}" value={customYear} onChange={(event) => setCustomYear(event.target.value)} /></label>
                    <button type="submit" disabled={currentCustomPending || debouncedQuery === ''}><Check aria-hidden="true" size={16} />保存自定义条目</button>
                    {currentCustomError ? <p className="form-error" role="alert">创建失败，标题、类型和年份仍保留。</p> : null}
                  </form>
                )}
              </div>
            ) : null}
            {selectionError ? <p className="form-error" role="alert">无法打开这个结果，搜索内容已保留。</p> : null}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

type SearchSectionProps = {
  heading: '本地影库' | 'TMDB'
  headingID: string
  items: MediaSearchResult[]
  pending: boolean
  error: boolean
  emptyLabel: string
  selectingID: string | null
  settledQuery: string
  onSelect: (item: MediaSearchResult, settledQuery: string) => Promise<void>
  onMoveFocus: (event: KeyboardEvent<HTMLButtonElement>, direction: -1 | 1) => void
}

function SearchSection({
  heading,
  headingID,
  items,
  pending,
  error,
  emptyLabel,
  selectingID,
  settledQuery,
  onSelect,
  onMoveFocus,
}: SearchSectionProps) {
  return (
    <section className="search-section" aria-labelledby={headingID}>
      <div className="search-section-heading">
        <h3 id={headingID}>{heading}</h3>
        {!pending && !error ? <span>{items.length}</span> : null}
      </div>
      {pending ? <SearchSkeleton /> : null}
      {error ? (
        <p className="search-section-error" role="alert" aria-label={`${heading}搜索失败`}>
          {heading === '本地影库' ? '无法搜索本地影库' : '无法搜索 TMDB'}
        </p>
      ) : null}
      {!pending && !error && items.length === 0 ? <p className="search-section-empty">{emptyLabel}</p> : null}
      {items.length > 0 ? (
        <ul className="search-result-list" aria-label={`${heading}搜索结果`}>
          {items.map((item) => {
            const key = resultKey(item)
            return (
              <li key={key}>
                <button
                  className="search-result"
                  data-search-result="true"
                  type="button"
                  aria-disabled={selectingID !== null}
                  aria-busy={selectingID === key}
                  onClick={() => void onSelect(item, settledQuery)}
                  onKeyDown={(event) => {
                    if (event.key === 'ArrowDown') onMoveFocus(event, 1)
                    if (event.key === 'ArrowUp') onMoveFocus(event, -1)
                  }}
                >
                  <MediaPoster item={item} compact />
                  <span className="result-source">{item.source === 'local' ? '本地影库' : 'TMDB'}</span>
                </button>
              </li>
            )
          })}
        </ul>
      ) : null}
    </section>
  )
}

function SearchSkeleton() {
  return <div className="search-skeleton" aria-label="正在搜索" />
}

function deduplicateRemote(local: MediaSearchResult[], remote: MediaSearchResult[]) {
  const seen = new Set(local.map(identityKey))
  return remote.filter((item) => !seen.has(identityKey(item)))
}

function identityKey(item: MediaSearchResult) {
  const tmdbID = item.externalId ?? item.tmdbId
  if (tmdbID) return `${item.mediaType}:tmdb:${tmdbID}`
  return `${item.mediaType}:${item.title.trim().toLocaleLowerCase()}:${item.year}`
}

function resultKey(item: MediaSearchResult) {
  return `${item.source}:${item.id}`
}

function resultButtons(container: HTMLDivElement | null) {
  if (!container) return []
  return Array.from(container.querySelectorAll<HTMLButtonElement>('[data-search-result="true"]:not([aria-disabled="true"])'))
}

function readRecentSearches(): string[] {
  try {
    const parsed: unknown = JSON.parse(window.sessionStorage.getItem(recentSearchesKey) ?? '[]')
    if (!Array.isArray(parsed)) return []
    const searches: string[] = []
    const seen = new Set<string>()
    for (const item of parsed) {
      if (typeof item !== 'string') continue
      const normalized = item.trim()
      const folded = normalized.toLocaleLowerCase()
      if (!normalized || seen.has(folded)) continue
      seen.add(folded)
      searches.push(normalized)
      if (searches.length === recentSearchLimit) break
    }
    return searches
  } catch {
    return []
  }
}

function writeRecentSearches(items: string[]) {
  try {
    window.sessionStorage.setItem(recentSearchesKey, JSON.stringify(items))
  } catch {
    // Storage can be unavailable or full; search remains fully usable.
  }
}

function clearRecentSearches() {
  try {
    window.sessionStorage.removeItem(recentSearchesKey)
  } catch {
    // Storage can be unavailable; the in-memory history is still cleared.
  }
}
