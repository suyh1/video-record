import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { MediaDetails, MediaSearchResult, RecordState } from '../../api/types'
import { CastStrip } from './CastStrip'
import { MediaHero } from './MediaHero'
import { MediaPoster } from './MediaPoster'

const signature = 'b'.repeat(64)
const posterURL = `/api/v1/public/tmdb/images/w342/tide-poster.png?expires=1784200000&signature=${signature}`
const backdropURL = `/api/v1/public/tmdb/images/w1280/tide-backdrop.png?expires=1784200000&signature=${signature}`
const profileURL = `/api/v1/public/tmdb/images/w300/cast-one.png?expires=1784200000&signature=${signature}`

describe('TMDB-backed media images', () => {
  it('keeps signed same-origin image proxy URLs unchanged', () => {
    const hero = render(
      <MediaHero
        media={{ ...media, posterPath: posterURL, backdropPath: backdropURL }}
        record={record}
        linker={null}
      />,
    )
    expect(screen.getByRole('img', { name: '潮汐档案 背景' })).toHaveAttribute('src', backdropURL)
    expect(screen.getByRole('img', { name: '潮汐档案 海报' })).toHaveAttribute('src', posterURL)
    hero.unmount()

    render(<CastStrip
      cast={[{ id: 1, name: '林见川', character: '顾潮', profilePath: profileURL, order: 0 }]}
      pending={false}
      error={false}
      linked
      onRetry={() => undefined}
    />)
    expect(screen.getByRole('img', { name: '林见川 饰 顾潮' })).toHaveAttribute('src', profileURL)
  })

  it('uses existing placeholders for direct or raw TMDB paths', () => {
    const poster: MediaSearchResult = {
      id: 'media-1', source: 'local', mediaType: 'movie', title: '潮汐档案', originalTitle: '',
      year: '2025', posterPath: 'https://image.tmdb.org/t/p/w342/tide-poster.jpg', status: 'none',
    }
    const posterView = render(<MediaPoster item={poster} />)
    expect(screen.queryByRole('img', { name: '潮汐档案 海报' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('潮汐档案 暂无海报')).toBeVisible()
    posterView.unmount()

    const hero = render(<MediaHero media={{ ...media, backdropPath: '/tide-backdrop.jpg' }} record={record} linker={null} />)
    expect(screen.queryByRole('img', { name: '潮汐档案 背景' })).not.toBeInTheDocument()
    hero.unmount()

    render(<CastStrip
      cast={[{ id: 1, name: '林见川', character: '顾潮', profilePath: 'https://IMAGE.TMDB.ORG./profile.jpg', order: 0 }]}
      pending={false}
      error={false}
      linked
      onRetry={() => undefined}
    />)
    expect(screen.queryByRole('img', { name: '林见川 饰 顾潮' })).not.toBeInTheDocument()
    expect(screen.getByText('林')).toBeVisible()
  })
})

const media: MediaDetails = {
  id: 'media-1', tmdbId: 1001, mediaType: 'tv', title: '潮汐档案', externalTitle: '',
  externalOverview: '', originalTitle: 'Tidal Archive', releaseDate: '2025-01-01', overview: '海岸档案。',
  posterPath: null, backdropPath: null, runtimeMinutes: 47, genres: ['剧情'],
}

const record: RecordState = {
  mediaId: 'media-1', status: 'none', rating: null, note: null, watchedAt: null,
  viewingMethod: null, version: 1,
}
