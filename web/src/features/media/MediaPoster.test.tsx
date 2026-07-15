import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { MediaSearchResult } from '../../api/types'
import { MediaPoster } from './MediaPoster'

const signature = 'a'.repeat(64)

function media(overrides: Partial<MediaSearchResult> = {}): MediaSearchResult {
  return {
    id: 'media-1',
    source: 'local',
    mediaType: 'movie',
    title: '花样年华',
    originalTitle: 'In the Mood for Love',
    year: '2000',
    posterPath: `/api/v1/public/tmdb/images/w342/mood.jpg?expires=4102444800&signature=${signature}`,
    status: 'completed',
    ...overrides,
  }
}

describe('MediaPoster', () => {
  it('uses a signed same-origin proxy URL unchanged with a meaningful alt', () => {
    render(<MediaPoster item={media()} />)

    expect(screen.getByRole('img', { name: '花样年华 海报' })).toHaveAttribute(
      'src',
      `/api/v1/public/tmdb/images/w342/mood.jpg?expires=4102444800&signature=${signature}`,
    )
  })

  it('hides an image that fails at runtime and exposes an accurate placeholder name', () => {
    render(<MediaPoster item={media()} />)

    fireEvent.error(screen.getByRole('img', { name: '花样年华 海报' }))

    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByLabelText('花样年华 暂无海报')).toHaveTextContent('花')
  })

  it('retries when the poster source or item changes after an image failure', async () => {
    const { rerender } = render(<MediaPoster item={media()} />)
    fireEvent.error(screen.getByRole('img', { name: '花样年华 海报' }))

    rerender(<MediaPoster item={media({ posterPath: `/api/v1/public/tmdb/images/w342/restored.jpg?signature=${signature}&expires=4102444800` })} />)
    await waitFor(() => expect(screen.getByRole('img', { name: '花样年华 海报' })).toHaveAttribute('src', expect.stringContaining('restored.jpg')))

    fireEvent.error(screen.getByRole('img', { name: '花样年华 海报' }))
    rerender(<MediaPoster item={media({ id: 'media-2', title: '一一', posterPath: `/api/v1/public/tmdb/images/w342/yiyi.jpg?expires=4102444800&signature=${signature}` })} />)
    await waitFor(() => expect(screen.getByRole('img', { name: '一一 海报' })).toBeVisible())
  })

  it('retries when item content changes while its identity and poster URL stay the same', async () => {
    const { rerender } = render(<MediaPoster item={media()} />)
    fireEvent.error(screen.getByRole('img', { name: '花样年华 海报' }))

    rerender(<MediaPoster item={media({ title: '花样年华（修复版）' })} />)

    await waitFor(() => expect(screen.getByRole('img', { name: '花样年华（修复版） 海报' })).toBeVisible())
  })

  it.each([
    '/raw-tmdb-poster.jpg',
    '/api/v1/public/tmdb/images/w342/unsigned.jpg',
    'https://image.tmdb.org/t/p/w342/direct.jpg',
  ])('refuses an unsafe TMDB image source: %s', (posterPath) => {
    render(<MediaPoster item={media({ posterPath })} />)

    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByLabelText('花样年华 暂无海报')).toBeVisible()
  })
})
