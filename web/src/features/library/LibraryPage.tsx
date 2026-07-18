import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { LoaderCircle, Search } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'

import { getCollectionItems, getCollections, getLibrary, getUserTags } from '../../api/client'
import type { RecordStatus } from '../../api/types'
import { MediaPoster } from '../media/MediaPoster'
import { CollectionManager } from '../collections/CollectionManager'

type LibraryFilter = RecordStatus | 'all'
type MediaTypeFilter = 'all' | 'movie' | 'tv'
type LibrarySort = 'updated' | 'title' | 'rating' | 'watched'

const filters: Array<{ value: LibraryFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'wishlist', label: '想看' },
  { value: 'watching', label: '在看' },
  { value: 'completed', label: '看过' },
  { value: 'dropped', label: '弃看' },
]

const mediaTypeFilters: Array<{ value: MediaTypeFilter; label: string }> = [
  { value: 'all', label: '全部类型' },
  { value: 'movie', label: '电影' },
  { value: 'tv', label: '剧集' },
]

const sortOptions: Array<{ value: LibrarySort; label: string }> = [
  { value: 'updated', label: '最近更新' },
  { value: 'watched', label: '最近观看' },
  { value: 'rating', label: '评分' },
  { value: 'title', label: '标题' },
]

const libraryPageSize = 40

export function LibraryPage({ onSearch }: { onSearch?: () => void }) {
  const [searchParams, setSearchParams] = useSearchParams()
  const [filter, setFilter] = useState<LibraryFilter>(() => {
    const status = searchParams.get('status')
    return filters.some((item) => item.value === status) ? status as LibraryFilter : 'all'
  })
  const [mediaType, setMediaType] = useState<MediaTypeFilter>(() => {
    const value = searchParams.get('mediaType')
    return value === 'movie' || value === 'tv' ? value : 'all'
  })
  const [sort, setSort] = useState<LibrarySort>(() => {
    const value = searchParams.get('sort')
    return sortOptions.some((item) => item.value === value) ? value as LibrarySort : 'updated'
  })
  const [query, setQuery] = useState(() => searchParams.get('q') ?? '')
  const [debouncedQuery, setDebouncedQuery] = useState(query.trim())
  const [tag, setTag] = useState(() => searchParams.get('tag') ?? '')
  const [genre, setGenre] = useState(() => searchParams.get('genre') ?? '')
  const [method, setMethod] = useState(() => searchParams.get('method') ?? '')
  const [selectedCollectionID, setSelectedCollectionID] = useState('')

  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedQuery(query.trim()), 300)
    return () => window.clearTimeout(timeout)
  }, [query])

  useEffect(() => {
    const next = new URLSearchParams()
    if (filter !== 'all') next.set('status', filter)
    if (mediaType !== 'all') next.set('mediaType', mediaType)
    if (sort !== 'updated') next.set('sort', sort)
    if (debouncedQuery) next.set('q', debouncedQuery)
    if (tag) next.set('tag', tag)
    if (genre) next.set('genre', genre)
    if (method) next.set('method', method)
    setSearchParams(next, { replace: true })
  }, [debouncedQuery, filter, genre, mediaType, method, setSearchParams, sort, tag])

  const library = useInfiniteQuery({
    queryKey: ['library', filter, mediaType, sort, debouncedQuery, tag, genre, method],
    queryFn: ({ pageParam, signal }) => getLibrary(filter, {
      ...(pageParam ? { cursor: pageParam } : {}),
      limit: libraryPageSize,
      mediaType,
      sort,
      ...(debouncedQuery ? { q: debouncedQuery } : {}),
      ...(tag ? { tag } : {}),
      ...(genre ? { genre } : {}),
      ...(method ? { method } : {}),
      signal,
    }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: !selectedCollectionID,
  })
  const collectionItems = useQuery({
    queryKey: ['collection-items', selectedCollectionID, filter],
    queryFn: ({ signal }) => getCollectionItems(selectedCollectionID, {
      status: filter,
      signal,
    }),
    enabled: Boolean(selectedCollectionID),
  })
  const collections = useQuery({ queryKey: ['collections'], queryFn: ({ signal }) => getCollections(signal) })
  const tags = useQuery({ queryKey: ['user-tags'], queryFn: ({ signal }) => getUserTags(signal) })
  const libraryItems = library.data?.pages.flatMap((page) => page.items) ?? []
  const selectedCollection = collections.data?.find((collection) => collection.id === selectedCollectionID)
  const displayedItems = selectedCollectionID
    ? collectionItems.data?.items ?? []
    : libraryItems
  const listPending = selectedCollectionID ? collectionItems.isPending : library.isPending
  const listError = selectedCollectionID ? collectionItems.isError : library.isError
  const listReady = selectedCollectionID ? Boolean(collectionItems.data) : Boolean(library.data)
  const reload = () => {
    if (selectedCollectionID) void collectionItems.refetch()
    else void library.refetch()
  }
  const headingMeta = selectedCollection
    ? `${selectedCollection.name}${filter !== 'all' ? ` · ${filters.find((item) => item.value === filter)?.label ?? ''}` : ''} · ${displayedItems.length} 部影视`
    : `${displayedItems.length} 部影视`

  return (
    <div className="page library-page">
      <header className="page-heading library-heading">
        <div>
          <p className="page-kicker">私人记录</p>
          <h1>影库</h1>
        </div>
        <p>{headingMeta}</p>
      </header>
      <CollectionManager
        mediaItems={selectedCollectionID ? displayedItems : libraryItems}
        selectedCollectionID={selectedCollectionID}
        onSelect={setSelectedCollectionID}
      />
      <label className="library-search-field">
        <span className="visually-hidden">影库内搜索</span>
        <Search aria-hidden="true" size={16} />
        <input
          type="search"
          placeholder="在影库中搜索标题"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      <div className="library-toolbar" role="group" aria-label="观看状态筛选">
        {filters.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={filter === item.value}
            onClick={() => setFilter(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div className="library-toolbar" role="group" aria-label="媒体类型筛选">
        {mediaTypeFilters.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={mediaType === item.value}
            onClick={() => setMediaType(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>
      <div className="library-toolbar" role="group" aria-label="排序">
        {sortOptions.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={sort === item.value}
            onClick={() => setSort(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>
      {tags.data?.tags.length ? (
        <div className="library-toolbar" role="group" aria-label="标签筛选">
          <button type="button" aria-pressed={!tag} onClick={() => setTag('')}>全部标签</button>
          {tags.data.tags.map((name) => (
            <button
              key={name}
              type="button"
              aria-pressed={tag === name}
              onClick={() => setTag(name)}
            >
              {name}
            </button>
          ))}
        </div>
      ) : null}
      {genre || method ? (
        <div className="library-toolbar" role="group" aria-label="统计下钻筛选">
          {genre ? (
            <button type="button" aria-pressed onClick={() => setGenre('')}>类型：{genre} ×</button>
          ) : null}
          {method ? (
            <button type="button" aria-pressed onClick={() => setMethod('')}>方式：{method} ×</button>
          ) : null}
        </div>
      ) : null}

      {listPending ? <LibrarySkeleton /> : null}
      {listError ? (
        <div className="library-message" role="alert">
          <p>无法读取影库，请稍后重试。</p>
          <button type="button" onClick={reload}>重新加载</button>
        </div>
      ) : null}
      {listReady && displayedItems.length > 0 ? (
        <>
          <div className="poster-grid">
            {displayedItems.map((item) => (
              <Link key={item.id} className="poster-link" to={`/media/${item.id}`}>
                <MediaPoster item={item} />
              </Link>
            ))}
          </div>
          {!selectedCollectionID && library.hasNextPage ? (
            <div className="library-load-more">
              <button
                type="button"
                disabled={library.isFetchingNextPage}
                onClick={() => void library.fetchNextPage()}
              >
                {library.isFetchingNextPage ? (
                  <>
                    <LoaderCircle className="loading-icon" aria-hidden="true" size={16} />
                    正在加载
                  </>
                ) : '加载更多'}
              </button>
            </div>
          ) : null}
        </>
      ) : null}
      {listReady && displayedItems.length === 0 ? (
        <div className="library-message empty-state">
          <Search aria-hidden="true" size={22} />
          <p>这个分类还没有记录</p>
          <button type="button" onClick={onSearch}>搜索影视</button>
        </div>
      ) : null}
    </div>
  )
}

function LibrarySkeleton() {
  return (
    <div className="poster-grid" aria-label="正在加载影库">
      {Array.from({ length: 6 }, (_, index) => <div key={index} className="skeleton library-poster-skeleton" />)}
    </div>
  )
}
