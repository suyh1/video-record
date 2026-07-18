import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { LoaderCircle, Search } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'

import { getCollectionItems, getCollections, getLibrary } from '../../api/client'
import type { RecordStatus } from '../../api/types'
import { MediaPoster } from '../media/MediaPoster'
import { CollectionManager } from '../collections/CollectionManager'

type LibraryFilter = RecordStatus | 'all'

const filters: Array<{ value: LibraryFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'wishlist', label: '想看' },
  { value: 'watching', label: '在看' },
  { value: 'completed', label: '看过' },
  { value: 'dropped', label: '弃看' },
]

const libraryPageSize = 40

export function LibraryPage({ onSearch }: { onSearch?: () => void }) {
  const [filter, setFilter] = useState<LibraryFilter>('all')
  const [selectedCollectionID, setSelectedCollectionID] = useState('')
  const library = useInfiniteQuery({
    queryKey: ['library', filter],
    queryFn: ({ pageParam, signal }) => getLibrary(filter, {
      ...(pageParam ? { cursor: pageParam } : {}),
      limit: libraryPageSize,
      signal,
    }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    enabled: !selectedCollectionID,
  })
  const collectionItems = useQuery({
    queryKey: ['collection-items', selectedCollectionID],
    queryFn: ({ signal }) => getCollectionItems(selectedCollectionID, { signal }),
    enabled: Boolean(selectedCollectionID),
  })
  const collections = useQuery({ queryKey: ['collections'], queryFn: ({ signal }) => getCollections(signal) })
  const libraryItems = library.data?.pages.flatMap((page) => page.items) ?? []
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

  return (
    <div className="page library-page">
      <header className="page-heading library-heading">
        <div>
          <p className="page-kicker">私人记录</p>
          <h1>影库</h1>
        </div>
        <p>{displayedItems.length} 部影视</p>
      </header>
      <CollectionManager
        mediaItems={selectedCollectionID ? displayedItems : libraryItems}
        selectedCollectionID={selectedCollectionID}
        onSelect={(collectionID) => {
          setSelectedCollectionID(collectionID)
          if (collectionID) setFilter('all')
        }}
      />
      <div className="library-toolbar" role="group" aria-label="观看状态筛选">
        {filters.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={filter === item.value}
            onClick={() => {
              setSelectedCollectionID('')
              setFilter(item.value)
            }}
          >
            {item.label}
          </button>
        ))}
      </div>

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
