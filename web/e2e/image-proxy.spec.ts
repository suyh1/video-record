import { expect, test, type Locator, type Page, type Request } from '@playwright/test'

import {
  admin,
  baseURL,
  expectImageLoaded,
  login,
  setSyntheticTMDBImageDelay,
  syntheticTMDBOrigin,
} from './support'

test('queues a slow detail image burst without dropping poster or cast portraits', async ({ page }) => {
  await page.route('**/api/v1/public/tmdb/highlights', (route) => route.fulfill({
    json: { items: [] },
    status: 200,
  }))
  await login(page)
  await setSyntheticTMDBImageDelay(page, 250)
  try {
    const imageStatuses: number[] = []
    page.on('response', (response) => {
      if (new URL(response.url()).pathname.startsWith('/api/v1/public/tmdb/images/')) {
        imageStatuses.push(response.status())
      }
    })

    await page.goto('/media/e2e-series')
    await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
    await expect.poll(() => page.locator('.media-details-page img').evaluateAll((images) => ({
      loaded: images.filter((image) => {
        const target = image as HTMLImageElement
        return target.complete && target.naturalWidth > 0 && target.naturalHeight > 0
      }).length,
      total: images.length,
    }))).toEqual({ loaded: 5, total: 5 })
    expect(imageStatuses).toHaveLength(5)
    expect(imageStatuses.every((status) => status === 200)).toBe(true)
  } finally {
    await setSyntheticTMDBImageDelay(page, 0)
  }
})

test('keeps authentication and media imagery behind the signed same-origin proxy', async ({ page }) => {
  const signedBackdrop = await readSignedBackdrop(page)
  await page.context().clearCookies()
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
  const forbiddenUpstreamRequests: string[] = []
  const proxyResponses = new Map<string, number>()
  page.on('request', (request) => {
    requests.push(request.url())
    if (isForbiddenUpstreamRequest(request)) forbiddenUpstreamRequests.push(request.url())
  })
  page.on('response', (response) => {
    const url = new URL(response.url())
    if (url.pathname.startsWith('/api/v1/public/tmdb/images/')) {
      proxyResponses.set(response.url(), response.status())
    }
  })

  await page.goto('/')
  await expect(page.getByRole('heading', { name: '登录 video-record' })).toBeVisible()
  await expectSignedProxyImage(page.locator('.auth-backdrop .backdrop-carousel__image.is-active'), proxyResponses)

  const csrfToken = await login(page)
  const homeHero = page.getByRole('region', { name: '首页主视觉' })
  await expect(homeHero).toHaveAttribute('data-backdrop-state', 'ready')
  await expectSignedProxyImage(homeHero.locator('.backdrop-carousel__image.is-active'), proxyResponses)

  await refreshSeededMediaImages(page, csrfToken)
  await page.goto('/library')
  await expect(page.getByRole('heading', { level: 1, name: '影库' })).toBeVisible()
  const libraryPosters = page.locator('.poster-frame img')
  await expect(libraryPosters).toHaveCount(2)
  for (const poster of await libraryPosters.all()) await expectSignedProxyImage(poster, proxyResponses)

  await page.goto('/media/e2e-series')
  await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
  const detailsBackdrop = page.locator('.media-hero-backdrop')
  await expectSignedProxyImage(detailsBackdrop, proxyResponses)

  expect(forbiddenUpstreamRequests, forbiddenUpstreamRequests.join('\n')).toEqual([])
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

async function expectSignedProxyImage(image: Locator, proxyResponses: Map<string, number>) {
  await expectImageLoaded(image)
  const source = await image.evaluate((element) => (element as HTMLImageElement).currentSrc)
  const url = new URL(source)
  expect(url.origin, source).toBe(baseURL)
  expect(url.pathname, source).toMatch(/^\/api\/v1\/public\/tmdb\/images\/(?:w300|w342|w780|w1280)\/[^/]+\.png$/)
  expect(url.searchParams.get('expires'), source).toMatch(/^\d+$/)
  expect(url.searchParams.get('signature'), source).toMatch(/^[a-f0-9]{64}$/)
  expect(proxyResponses.get(source), source).toBe(200)
}

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

function isForbiddenUpstreamRequest(request: Request) {
  const url = new URL(request.url())
  const hostname = url.hostname.toLowerCase()
  return url.origin === syntheticTMDBOrigin
    || hostname === 'image.tmdb.org'
    || hostname.endsWith('.image.tmdb.org')
    || hostname === 'api.themoviedb.org'
    || hostname.endsWith('.api.themoviedb.org')
}
