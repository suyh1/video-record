import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import type { MediaDetails } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { TMDBLinker } from './TMDBLinker'

describe('TMDBLinker', () => {
  it('does not offer another link when a custom title already has a TMDB identity', () => {
    const media: MediaDetails = {
      id: 'linked-custom',
      tmdbId: 1001,
      mediaType: 'tv',
      title: '自定义片名',
      overview: '本地简介',
      externalTitle: '',
      externalOverview: '',
      originalTitle: '',
      releaseDate: '2025',
      posterPath: null,
      backdropPath: null,
      runtimeMinutes: 0,
      genres: [],
    }

    renderWithQueryClient(<TMDBLinker media={media} />)

    expect(screen.queryByRole('button', { name: '关联 TMDB' })).not.toBeInTheDocument()
  })
})
