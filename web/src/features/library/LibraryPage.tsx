import { useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'

import { getLibrary } from '../../api/client'
import type { RecordStatus } from '../../api/types'
import { MediaPoster } from '../media/MediaPoster'

type LibraryFilter = RecordStatus | 'all'

const filters: Array<{ value: LibraryFilter; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'wishlist', label: '想看' },
  { value: 'watching', label: '在看' },
  { value: 'completed', label: '看过' },
  { value: 'dropped', label: '弃看' },
]

export function LibraryPage({ onSearch }: { onSearch?: () => void }) {
  const [filter, setFilter] = useState<LibraryFilter>('all')
  const library = useQuery({
    queryKey: ['library', filter],
    queryFn: ({ signal }) => getLibrary(filter, signal),
  })

  return (
    <div className="page library-page">
      <header className="page-heading library-heading">
        <div>
          <p className="page-kicker">私人记录</p>
          <h1>影库</h1>
        </div>
        <p>{library.data?.items.length ?? 0} 部影视</p>
      </header>
      <div className="library-toolbar" aria-label="影库筛选">
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

      {library.isPending ? <LibrarySkeleton /> : null}
      {library.isError ? (
        <div className="library-message" role="alert">
          <p>无法读取影库，请稍后重试。</p>
          <button type="button" onClick={() => void library.refetch()}>重新加载</button>
        </div>
      ) : null}
      {library.data && library.data.items.length > 0 ? (
        <div className="poster-grid">
          {library.data.items.map((item) => (
            <Link key={item.id} className="poster-link" to={`/media/${item.id}`}>
              <MediaPoster item={item} />
            </Link>
          ))}
        </div>
      ) : null}
      {library.data?.items.length === 0 ? (
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
