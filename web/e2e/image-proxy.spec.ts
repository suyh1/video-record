import { expect, test, type Page, type Request } from '@playwright/test'

import { admin, baseURL, expectImageLoaded, login } from './support'

test('keeps authentication and media imagery behind the signed same-origin proxy', async ({ page }) => {
  const signedBackdrop = await readSignedBackdrop(page)
  await page.context().clearCookies()
  await page.addInitScript(() => window.sessionStorage.clear())
  await page.route('**/api/v1/public/tmdb/highlights', (route) => route.fulfill({
    json: {
      items: [{
        id: 1001,
        mediaType: 'tv',
        title: '潮汐档案',
        originalTitle: 'Tidal Archive',
        year: '2025',
        overview: '合成 TMDB 登录背景',
        backdropURL: signedBackdrop,
      }],
    },
    status: 200,
  }))

  const requests: string[] = []
  const directTMDBRequests: string[] = []
  const proxyResponses = new Map<string, number>()
  page.on('request', (request) => {
    requests.push(request.url())
    if (isDirectTMDBRequest(request)) directTMDBRequests.push(request.url())
  })
  page.on('response', (response) => {
    const url = new URL(response.url())
    if (url.pathname.startsWith('/api/v1/public/tmdb/images/')) {
      proxyResponses.set(response.url(), response.status())
    }
  })

  await page.goto('/')
  await expect(page.getByRole('heading', { name: '登录 video-record' })).toBeVisible()
  await expectImageLoaded(page.locator('.auth-backdrop .backdrop-carousel__image.is-active'))

  const csrfToken = await login(page)
  const homeHero = page.getByRole('region', { name: '首页主视觉' })
  await expect(homeHero).toHaveAttribute('data-backdrop-state', 'ready')
  await expectImageLoaded(homeHero.locator('.backdrop-carousel__image.is-active'))

  await refreshSeededMediaImages(page, csrfToken)
  await page.goto('/library')
  await expect(page.getByRole('heading', { level: 1, name: '影库' })).toBeVisible()
  const libraryPosters = page.locator('.poster-frame img')
  await expect(libraryPosters).toHaveCount(2)
  for (const poster of await libraryPosters.all()) await expectImageLoaded(poster)

  await page.goto('/media/e2e-series')
  await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
  const detailsBackdrop = page.locator('.media-hero-backdrop')
  await expectImageLoaded(detailsBackdrop)

  expect(directTMDBRequests, directTMDBRequests.join('\n')).toEqual([])
  const proxyRequests = requests.filter((value) => new URL(value).pathname.startsWith('/api/v1/public/tmdb/images/'))
  expect(proxyRequests.length).toBeGreaterThan(0)
  for (const value of proxyRequests) {
    const url = new URL(value)
    expect(url.origin).toBe(baseURL)
    expect(url.pathname).toMatch(/^\/api\/v1\/public\/tmdb\/images\/(?:w300|w342|w780|w1280)\/[^/]+\.png$/)
    expect(url.searchParams.get('expires')).toMatch(/^\d+$/)
    expect(url.searchParams.get('signature')).toMatch(/^[a-f0-9]{64}$/)
    expect(proxyResponses.get(value), value).toBe(200)
  }
})

async function refreshSeededMediaImages(page: Page, csrfToken: string) {
  const media = [
    { externalID: 2002, id: 'e2e-movie', mediaType: 'movie' },
    { externalID: 1001, id: 'e2e-series', mediaType: 'tv' },
  ]
  for (const item of media) {
    const response = await page.context().request.post(
      `${baseURL}/api/v1/media/${item.id}/tmdb/${item.mediaType}/${item.externalID}`,
      {
        headers: {
          'Idempotency-Key': `refresh-${item.id}-images`,
          Origin: baseURL,
          'X-CSRF-Token': csrfToken,
        },
      },
    )
    expect(response.ok()).toBeTruthy()
  }
}

async function readSignedBackdrop(page: Page) {
  const loginResponse = await page.context().request.post(`${baseURL}/api/v1/auth/login`, {
    data: admin,
    headers: { Origin: baseURL },
  })
  expect(loginResponse.ok()).toBeTruthy()
  const response = await page.context().request.get(`${baseURL}/api/v1/tmdb/tv/1001`)
  expect(response.ok()).toBeTruthy()
  const body = await response.json() as { backdropPath: string }
  expect(body.backdropPath).toMatch(/^\/api\/v1\/public\/tmdb\/images\/w1280\//)
  return body.backdropPath
}

function isDirectTMDBRequest(request: Request) {
  const hostname = new URL(request.url()).hostname.toLowerCase()
  return hostname === 'image.tmdb.org'
    || hostname.endsWith('.image.tmdb.org')
    || hostname === 'api.themoviedb.org'
    || hostname.endsWith('.api.themoviedb.org')
}
