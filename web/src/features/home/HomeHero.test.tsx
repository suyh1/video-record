import { act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { sampleMediaAccent } from '../../lib/mediaAccent'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { HomeHero } from './HomeHero'

vi.mock('../../lib/mediaAccent', () => ({ sampleMediaAccent: vi.fn() }))

afterEach(() => {
  vi.clearAllMocks()
  vi.unstubAllGlobals()
  document.documentElement.removeAttribute('data-theme')
})

describe('HomeHero', () => {
  it('shows a hero skeleton without announcing an empty state while data is pending', () => {
    renderWithQueryClient(
      <HomeHero
        isError={false}
        isLoading
        items={[]}
        onRetry={vi.fn()}
      />,
    )

    const hero = screen.getByRole('region', { name: '首页主视觉' })
    expect(hero).toHaveClass('is-loading')
    expect(hero).toHaveAttribute('data-backdrop-state', 'loading')
    expect(screen.getByLabelText('正在加载首页主视觉')).toBeVisible()
    expect(screen.queryByText('还没有可展示的影视背景')).not.toBeInTheDocument()
  })

  it('keeps an accessible white empty hero with a search action', async () => {
    const onSearch = vi.fn()
    const user = userEvent.setup()

    const { container } = renderWithQueryClient(
      <HomeHero
        isError={false}
        isLoading={false}
        items={[]}
        onRetry={vi.fn()}
        onSearch={onSearch}
      />,
    )

    const hero = screen.getByRole('region', { name: '首页主视觉' })
    expect(hero).toHaveClass('home-hero', 'is-empty')
    expect(hero).toHaveAttribute('data-backdrop-state', 'empty')
    expect(screen.getByRole('heading', { level: 1, name: '首页' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: '搜索影视' }))
    expect(onSearch).toHaveBeenCalledOnce()
    expect(container.querySelector('img')).toBeNull()
  })

  it('reveals a decoded private movie with record CTA and carousel progress', async () => {
    const images = installDecodedImageMock()
    const item = privateMovie()

    const { container } = renderWithQueryClient(
      <MemoryRouter>
        <HomeHero
          isError={false}
          isLoading={false}
          items={[item]}
          onRetry={vi.fn()}
          onSearch={vi.fn()}
        />
      </MemoryRouter>,
    )

    expect(images).toHaveLength(1)
    await act(async () => {
      images[0]!.resolveDecode()
      await Promise.resolve()
    })

    const hero = screen.getByRole('region', { name: '首页主视觉' })
    expect(hero).toHaveClass('has-backdrop')
    expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
    expect(screen.getByRole('heading', { level: 1, name: '花样年华' })).toBeVisible()
    expect(screen.getByText('2000 · 电影')).toBeVisible()
    expect(screen.getByText('两个邻居在狭窄走廊里逐渐靠近。')).toBeVisible()
    expect(screen.getByRole('link', { name: '查看记录' })).toHaveAttribute('href', '/media/movie-1')
    expect(screen.getByText('1 / 1')).toBeVisible()
    expect(container.querySelector('.backdrop-carousel__image.is-active')).toHaveAttribute('src', item.backdropURL)
  })

  it('keeps automatic carousel content out of live regions', async () => {
    const images = installDecodedImageMock()

    const { container } = renderWithQueryClient(
      <MemoryRouter>
        <HomeHero
          isError={false}
          isLoading={false}
          items={[privateMovie()]}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )
    await act(async () => {
      images[0]!.resolveDecode()
      await Promise.resolve()
    })

    expect(container.querySelector('.home-hero__content')).not.toHaveAttribute('aria-live')
  })

  it('does not cover a decoded private hero while supplemental data is still pending', async () => {
    const images = installDecodedImageMock()

    renderWithQueryClient(
      <MemoryRouter>
        <HomeHero
          isError={false}
          isLoading
          items={[privateMovie()]}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )
    await act(async () => {
      images[0]!.resolveDecode()
      await Promise.resolve()
    })

    expect(screen.getByRole('heading', { level: 1, name: '花样年华' })).toBeVisible()
    expect(screen.queryByLabelText('正在加载首页主视觉')).not.toBeInTheDocument()
  })

  it('falls back to the brand accent when media color sampling fails', async () => {
    vi.mocked(sampleMediaAccent).mockReturnValue(null)
    const images = installDecodedImageMock()

    renderWithQueryClient(
      <MemoryRouter>
        <HomeHero
          isError={false}
          isLoading={false}
          items={[privateMovie()]}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )
    await act(async () => {
      images[0]!.resolveDecode()
      await Promise.resolve()
    })

    const hero = screen.getByRole('region', { name: '首页主视觉' })
    expect(sampleMediaAccent).toHaveBeenCalledWith(images[0])
    expect(hero.style.getPropertyValue('--media-accent')).toBe('var(--primary)')
  })

  it('keeps hero retry separate from the rest of the home page', async () => {
    const onRetry = vi.fn()
    const user = userEvent.setup()

    renderWithQueryClient(
      <HomeHero
        isError
        isLoading={false}
        items={[]}
        onRetry={onRetry}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('主视觉暂时无法加载')
    await user.click(screen.getByRole('button', { name: '重试主视觉' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('reports an empty hero and removes broken images when every candidate fails to decode', async () => {
    document.documentElement.setAttribute('data-theme', 'dark')
    const images = installDecodedImageMock()
    const onBackdropStateChange = vi.fn()
    const first = privateMovie()
    const second = { ...privateMovie(), id: 205, title: '第二个失败背景', backdropURL: privateMovie().backdropURL.replace('in-the-mood', 'second') }

    const { container } = renderWithQueryClient(
      <MemoryRouter>
        <HomeHero
          isError={false}
          isLoading={false}
          items={[first, second]}
          onBackdropStateChange={onBackdropStateChange}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )
    await act(async () => {
      images[0]!.rejectDecode()
      await Promise.resolve()
    })
    await act(async () => {
      images[1]!.rejectDecode()
      await Promise.resolve()
    })

    const hero = screen.getByRole('region', { name: '首页主视觉' })
    expect(hero).toHaveClass('is-empty')
    expect(hero).toHaveAttribute('data-backdrop-state', 'empty')
    expect(container.querySelector('.home-hero img')).toBeNull()
    expect(onBackdropStateChange).toHaveBeenLastCalledWith('empty')
  })

  it('links a watching series to its real next episode when the catalog is available', async () => {
    const images = installDecodedImageMock()
    server.use(
      http.get('*/api/v1/tmdb/tv/1399', () => HttpResponse.json(tvDetails())),
      http.get('*/api/v1/records/series-1/rounds/current', ({ request }) => {
        const seasonNumber = Number(new URL(request.url).searchParams.get('seasonNumber'))
        return HttpResponse.json({
          roundId: `round-${seasonNumber}`,
          mediaId: 'series-1',
          seasonNumber,
          roundNumber: 1,
          status: seasonNumber === 2 ? 'watching' : 'completed',
          rating: null,
          note: null,
          viewingMethod: null,
          watchedAt: null,
          participantIds: [],
          version: 1,
          profileVersion: 1,
        })
      }),
      http.get('*/api/v1/records/series-1/progress', () => HttpResponse.json({
        roundId: 'round-2', mediaId: 'series-1', seasonNumber: 2, status: 'watching', version: 1,
        watchedEpisodes: 3, totalEpisodes: 4, lastWatched: null, nextEpisode: null,
        episodes: [1, 2, 3].map((episodeNumber) => ({
          id: `episode-${episodeNumber}`, sourceId: String(200 + episodeNumber), seasonId: 'season-2',
          seasonNumber: 2, episodeNumber, absoluteNumber: episodeNumber + 4, name: `第${episodeNumber}集`,
          watched: true, startedAt: null, watchedAt: '2026-07-12T12:00:00Z',
        })),
      })),
      http.get('*/api/v1/tmdb/tv/1399/season/2', () => HttpResponse.json({
        id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2,
        episodes: [1, 2, 3, 4].map((episodeNumber) => ({
          id: 200 + episodeNumber, name: episodeNumber === 4 ? '第四集' : `第${episodeNumber}集`, overview: '',
          airDate: '2026-07-01', seasonNumber: 2, episodeNumber, runtime: 45, stillPath: '',
        })),
      })),
    )

    renderWithQueryClient(
      <MemoryRouter>
        <HomeHero
          isError={false}
          isLoading={false}
          items={[privateTV()]}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )
    await act(async () => {
      images[0]!.resolveDecode()
      await Promise.resolve()
    })

    expect(await screen.findByRole('link', { name: '继续 S02E04' })).toHaveAttribute('href', '/media/series-1')
    expect(screen.getByText('下一集 · 第四集')).toBeVisible()
  })
})

type DeferredImage = {
  rejectDecode: () => void
  resolveDecode: () => void
}

function installDecodedImageMock() {
  const images: DeferredImage[] = []
  class TestImage {
    fetchPriority = 'auto'
    onerror: ((event: Event) => void) | null = null
    onload: ((event: Event) => void) | null = null
    src = ''
    private rejectPromise!: (reason: Error) => void
    private resolvePromise!: () => void
    private readonly decodePromise = new Promise<void>((resolve, reject) => {
      this.resolvePromise = resolve
      this.rejectPromise = reject
    })
    decode = vi.fn(() => this.decodePromise)
    resolveDecode = () => this.resolvePromise()
    rejectDecode = () => this.rejectPromise(new Error('decode failed'))

    constructor() {
      images.push(this)
    }
  }
  vi.stubGlobal('Image', TestImage)
  return images
}

function privateMovie() {
  return {
    id: 204,
    mediaType: 'movie' as const,
    title: '花样年华',
    originalTitle: 'In the Mood for Love',
    year: '2000',
    overview: '两个邻居在狭窄走廊里逐渐靠近。',
    backdropURL: `/api/v1/public/tmdb/images/w1280/in-the-mood.jpg?expires=4102444800&signature=${'a'.repeat(64)}`,
    localItem: {
      id: 'movie-1',
      tmdbId: 204,
      source: 'local' as const,
      mediaType: 'movie' as const,
      title: '花样年华',
      originalTitle: 'In the Mood for Love',
      year: '2000',
      posterPath: null,
      status: 'completed' as const,
    },
  }
}

function privateTV() {
  return {
    id: 1399,
    mediaType: 'tv' as const,
    title: '漫长的季节',
    originalTitle: 'The Long Season',
    year: '2023',
    overview: '一座小城里的漫长回声。',
    backdropURL: `/api/v1/public/tmdb/images/w1280/long-season.jpg?expires=4102444800&signature=${'c'.repeat(64)}`,
    localItem: {
      id: 'series-1', tmdbId: 1399, source: 'local' as const, mediaType: 'tv' as const,
      title: '漫长的季节', originalTitle: 'The Long Season', year: '2023', posterPath: null,
      status: 'watching' as const,
    },
  }
}

function tvDetails() {
  return {
    id: 1399, name: '漫长的季节', originalName: 'The Long Season', firstAirDate: '2023-04-22',
    posterPath: '', backdropPath: privateTV().backdropURL, overview: '一座小城里的漫长回声。',
    numberOfSeasons: 2, numberOfEpisodes: 8, episodeRuntime: [45], genres: ['剧情'],
    seasons: [
      { id: 11, name: '第 1 季', overview: '', posterPath: '', airDate: '2023-04-22', seasonNumber: 1, episodeCount: 4 },
      { id: 12, name: '第 2 季', overview: '', posterPath: '', airDate: '2026-07-01', seasonNumber: 2, episodeCount: 4 },
    ],
  }
}
