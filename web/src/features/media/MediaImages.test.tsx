import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { MediaDetails, MediaSearchResult, RecordState } from '../../api/types'
import { sampleMediaAccent } from '../../lib/mediaAccent'
import { MediaHero } from './MediaHero'
import { MediaPoster } from './MediaPoster'

vi.mock('../../lib/mediaAccent', () => ({ sampleMediaAccent: vi.fn() }))

const signature = 'b'.repeat(64)
const posterURL = `/api/v1/public/tmdb/images/w342/tide-poster.png?expires=1784200000&signature=${signature}`
const backdropURL = `/api/v1/public/tmdb/images/w1280/tide-backdrop.png?expires=1784200000&signature=${signature}`

describe('TMDB-backed media images', () => {
  beforeEach(() => vi.mocked(sampleMediaAccent).mockReset())

  it('keeps signed same-origin image proxy URLs unchanged', () => {
    const { container } = render(
      <MediaHero
        media={{ ...media, posterPath: posterURL, backdropPath: backdropURL }}
        record={record}
        linker={null}
      />,
    )
    expect(container.querySelector('.media-hero-backdrop')).toHaveAttribute('src', backdropURL)
    expect(screen.getByRole('img', { name: '潮汐档案 海报' })).toHaveAttribute('src', posterURL)

    const hero = container.querySelector('.media-hero')!
    expect(hero).toHaveAttribute('data-backdrop-state', 'loading')
    expect(hero).not.toHaveClass('has-backdrop')
    expect(container.querySelector('.media-hero-backdrop')).toHaveAttribute('alt', '')
    expect(screen.queryByRole('img', { name: '潮汐档案 背景' })).not.toBeInTheDocument()

    fireEvent.load(container.querySelector<HTMLImageElement>('.media-hero-backdrop')!)
    expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
    expect(hero).toHaveClass('has-backdrop')
  })

  it('remounts a ready same-source backdrop when its title changes and ignores the old load event', () => {
    vi.mocked(sampleMediaAccent).mockReturnValue('oklch(0.610 0.130 210.0)')
    const { container, rerender } = render(
      <MediaHero media={{ ...media, title: '标题 A', backdropPath: backdropURL }} record={record} linker={null} />,
    )
    const hero = container.querySelector<HTMLElement>('.media-hero')!
    const firstBackdrop = container.querySelector<HTMLImageElement>('.media-hero-backdrop')!
    fireEvent.load(firstBackdrop)
    expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
    expect(hero.style.getPropertyValue('--media-accent')).toBe('oklch(0.610 0.130 210.0)')

    rerender(<MediaHero media={{ ...media, title: '标题 B', backdropPath: backdropURL }} record={record} linker={null} />)
    const secondBackdrop = container.querySelector<HTMLImageElement>('.media-hero-backdrop')!
    expect(secondBackdrop).not.toBe(firstBackdrop)
    expect(hero).toHaveAttribute('data-backdrop-state', 'loading')
    expect(screen.getByRole('heading', { level: 1, name: '标题 B' })).toBeVisible()
    expect(hero.style.getPropertyValue('--media-accent')).toBe('var(--brand)')

    fireEvent.load(firstBackdrop)
    expect(hero).toHaveAttribute('data-backdrop-state', 'loading')

    vi.mocked(sampleMediaAccent).mockReturnValue('oklch(0.580 0.120 28.0)')
    fireEvent.load(secondBackdrop)
    expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
    expect(hero.style.getPropertyValue('--media-accent')).toBe('oklch(0.580 0.120 28.0)')
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

    const { container } = render(<MediaHero media={{ ...media, backdropPath: '/tide-backdrop.jpg' }} record={record} linker={null} />)
    expect(container.querySelector('.media-hero-backdrop')).not.toBeInTheDocument()
  })

  it('removes a failed backdrop and restores the brand accent without leaving a broken image', () => {
    vi.mocked(sampleMediaAccent).mockReturnValue('oklch(0.620 0.140 210.0)')
    const { container } = render(
      <MediaHero media={{ ...media, backdropPath: backdropURL }} record={record} linker={null} />,
    )
    const hero = container.querySelector<HTMLElement>('.media-hero')!
    const backdrop = container.querySelector<HTMLImageElement>('.media-hero-backdrop')!

    fireEvent.load(backdrop)
    expect(hero.style.getPropertyValue('--media-accent')).toBe('oklch(0.620 0.140 210.0)')
    fireEvent.error(backdrop)

    expect(container.querySelector('.media-hero-backdrop')).not.toBeInTheDocument()
    expect(hero).not.toHaveClass('has-backdrop')
    expect(hero).toHaveAttribute('data-backdrop-state', 'failed')
    expect(hero.style.getPropertyValue('--media-accent')).toBe('var(--brand)')
  })

  it('falls back to the brand accent when backdrop sampling is blocked', () => {
    vi.mocked(sampleMediaAccent).mockReturnValue(null)
    const { container } = render(
      <MediaHero media={{ ...media, backdropPath: backdropURL }} record={record} linker={null} />,
    )

    fireEvent.load(container.querySelector<HTMLImageElement>('.media-hero-backdrop')!)

    expect(sampleMediaAccent).toHaveBeenCalledOnce()
    expect(container.querySelector<HTMLElement>('.media-hero')!.style.getPropertyValue('--media-accent')).toBe('var(--brand)')
  })

  it('retries a failed backdrop when the same item image or title changes', () => {
    const restoredURL = `/api/v1/public/tmdb/images/w1280/restored.png?expires=1784200000&signature=${signature}`
    const { container, rerender } = render(
      <MediaHero media={{ ...media, backdropPath: backdropURL }} record={record} linker={null} />,
    )
    fireEvent.error(container.querySelector<HTMLImageElement>('.media-hero-backdrop')!)

    rerender(<MediaHero media={{ ...media, backdropPath: restoredURL }} record={record} linker={null} />)
    expect(container.querySelector('.media-hero-backdrop')).toHaveAttribute('src', restoredURL)

    fireEvent.error(container.querySelector<HTMLImageElement>('.media-hero-backdrop')!)
    rerender(<MediaHero media={{ ...media, title: '潮汐档案：重映', backdropPath: restoredURL }} record={record} linker={null} />)
    expect(container.querySelector('.media-hero-backdrop')).toHaveAttribute('src', restoredURL)
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
