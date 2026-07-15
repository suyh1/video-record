import { describe, expect, it } from 'vitest'

import { mediaImageURL } from './mediaImage'

const signature = 'a'.repeat(64)
const signedImage = `/api/v1/public/tmdb/images/w1280/tide-backdrop.png?expires=1784200000&signature=${signature}`

describe('mediaImageURL', () => {
  it('preserves a signed same-origin TMDB proxy URL', () => {
    expect(mediaImageURL(signedImage)).toBe(signedImage)
  })

  it('preserves an explicit custom cross-origin HTTP image', () => {
    expect(mediaImageURL('https://media.example.test/custom/poster.jpg')).toBe('https://media.example.test/custom/poster.jpg')
    expect(mediaImageURL('http://192.0.2.10/poster.png')).toBe('http://192.0.2.10/poster.png')
  })

  it.each([
    '/poster.jpg',
    '/api/v1/auth/me',
    `${window.location.origin}/api/v1/auth/me`,
    '//image.tmdb.org/t/p/w342/poster.jpg',
    'https://image.tmdb.org/t/p/w342/poster.jpg',
    'https://IMAGE.TMDB.ORG./t/p/w342/poster.jpg',
    'https://image.tmdb。org/t/p/w342/poster.jpg',
    'https://sub.image.tmdb.org/t/p/w342/poster.jpg',
    'javascript:alert(1)',
  ])('rejects an untrusted image source: %s', (source) => {
    expect(mediaImageURL(source)).toBeNull()
  })

  it('rejects an unsigned lookalike proxy path', () => {
    expect(mediaImageURL('/api/v1/public/tmdb/images/w1280/tide-backdrop.png')).toBeNull()
    expect(mediaImageURL(`/api/v1/public/tmdb/images/original/tide-backdrop.png?expires=1784200000&signature=${signature}`)).toBeNull()
  })
})
