import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link2, LoaderCircle, Search, X } from 'lucide-react'
import { useEffect, useState } from 'react'

import { linkMediaToTMDB, searchTMDB } from '../../api/client'
import type { MediaDetails, MediaSearchResult } from '../../api/types'

type TMDBLinkerProps = {
  media: MediaDetails
}

export function TMDBLinker({ media }: TMDBLinkerProps) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState(media.title)
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [linked, setLinked] = useState(false)
  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedQuery(query.trim()), 300)
    return () => window.clearTimeout(timeout)
  }, [query])
  const search = useQuery({
    queryKey: ['media-link-search', media.id, debouncedQuery],
    queryFn: ({ signal }) => searchTMDB(debouncedQuery, signal),
    enabled: open && debouncedQuery.length > 0,
  })
  const results = (search.data ?? []).filter((item) => item.mediaType === media.mediaType)
  const mutation = useMutation({
    mutationFn: (item: MediaSearchResult) => linkMediaToTMDB(media.id, item),
    onSuccess: (item) => {
      queryClient.setQueryData(['media', media.id], item)
      setLinked(true)
      setOpen(false)
    },
  })

  if (linked) return <p className="tmdb-link-success" role="status">已关联 TMDB，个人记录保持不变</p>
  if (media.externalTitle) return null

  return (
    <div className="tmdb-linker">
      {!open ? (
        <button type="button" onClick={() => setOpen(true)}><Link2 aria-hidden="true" size={16} />关联 TMDB</button>
      ) : (
        <div className="tmdb-link-panel">
          <div className="tmdb-link-heading">
            <strong>关联 TMDB 元数据</strong>
            <button type="button" aria-label="关闭 TMDB 关联" onClick={() => setOpen(false)}><X aria-hidden="true" size={17} /></button>
          </div>
          <label className="tmdb-link-search">
            <Search aria-hidden="true" size={17} />
            <input
              autoFocus
              type="search"
              aria-label="搜索 TMDB 关联"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <div className="tmdb-link-results" aria-live="polite">
            {query && (!debouncedQuery || search.isPending) ? <div className="skeleton tmdb-link-skeleton" aria-label="正在搜索 TMDB" /> : null}
            {results.map((item) => (
              <button
                key={item.id}
                type="button"
                aria-label={`关联 TMDB：${item.title}（${item.year || '年份未知'}）`}
                disabled={mutation.isPending}
                onClick={() => mutation.mutate(item)}
              >
                <span><strong>{item.title}</strong><small>{item.originalTitle}</small></span>
                <span>{item.year || '年份未知'}</span>
                {mutation.isPending ? <LoaderCircle className="loading-icon" aria-hidden="true" size={16} /> : <Link2 aria-hidden="true" size={16} />}
              </button>
            ))}
            {debouncedQuery && !search.isPending && !search.isError && results.length === 0 ? <p>没有找到同类型的 TMDB 条目</p> : null}
            {search.isError ? <p role="alert">TMDB 搜索暂时不可用，搜索内容已保留。</p> : null}
            {mutation.isError ? <p role="alert">关联失败，原有条目和个人记录保持不变。</p> : null}
          </div>
        </div>
      )}
    </div>
  )
}
