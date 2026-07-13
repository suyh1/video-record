import * as Dialog from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { Search, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'

import { searchLocalMedia, searchTMDB } from '../../api/client'
import type { MediaSearchResult } from '../../api/types'
import { MediaPoster } from '../media/MediaPoster'

type SearchDialogProps = {
  open: boolean
  onClose: () => void
  onSelect: (item: MediaSearchResult) => void | Promise<void>
}

export function SearchDialog({ open, onClose, onSelect }: SearchDialogProps) {
  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [selectingID, setSelectingID] = useState<string | null>(null)
  const [selectionError, setSelectionError] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedQuery(query.trim()), 300)
    return () => window.clearTimeout(timeout)
  }, [query])

  const enabled = open && debouncedQuery.length > 0
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
  const results = useMemo(() => mergeResults(local.data ?? [], remote.data ?? []), [local.data, remote.data])

  const selectResult = async (item: MediaSearchResult) => {
    setSelectingID(item.id)
    setSelectionError(false)
    try {
      await onSelect(item)
    } catch {
      setSelectionError(true)
    } finally {
      setSelectingID(null)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={(nextOpen) => !nextOpen && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="search-overlay" />
        <Dialog.Content className="search-dialog" onOpenAutoFocus={(event) => {
          event.preventDefault()
          inputRef.current?.focus()
        }}>
          <div className="search-dialog-heading">
            <Dialog.Title>搜索影视</Dialog.Title>
            <Dialog.Close className="icon-button" aria-label="关闭搜索">
              <X aria-hidden="true" size={20} />
            </Dialog.Close>
          </div>
          <div className="search-dialog-input">
            <Search aria-hidden="true" size={18} />
            <input
              ref={inputRef}
              type="search"
              aria-label="搜索影视"
              placeholder="输入电影或剧集名称"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </div>
          <div className="search-results" aria-live="polite">
            {!query ? <p className="search-guidance">本地记录会先显示，随后合并 TMDB 结果。</p> : null}
            {query && !debouncedQuery ? <SearchSkeleton /> : null}
            {enabled && local.isPending ? <SearchSkeleton /> : null}
            {results.map((item) => (
              <button
                key={`${item.source}-${item.id}`}
                className="search-result"
                type="button"
                disabled={selectingID !== null}
                onClick={() => void selectResult(item)}
              >
                <MediaPoster item={item} compact />
                <span className="result-source">{item.source === 'local' ? '本地影库' : 'TMDB'}</span>
              </button>
            ))}
            {enabled && !local.isPending && !remote.isPending && results.length === 0 ? (
              <p className="search-guidance">没有找到匹配的电影或剧集</p>
            ) : null}
            {local.isError && remote.isError ? <p className="form-error">搜索暂时不可用，请稍后重试。</p> : null}
            {selectionError ? <p className="form-error" role="alert">无法打开这个结果，搜索内容已保留。</p> : null}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function SearchSkeleton() {
  return <div className="search-skeleton" aria-label="正在搜索" />
}

function mergeResults(local: MediaSearchResult[], remote: MediaSearchResult[]) {
  const seen = new Set(local.map(identityKey))
  return [...local, ...remote.filter((item) => !seen.has(identityKey(item)))]
}

function identityKey(item: MediaSearchResult) {
  if (item.externalId) return `${item.mediaType}:tmdb:${item.externalId}`
  return `${item.mediaType}:${item.title.trim().toLocaleLowerCase()}:${item.year}`
}
