import AxeBuilder from '@axe-core/playwright'
import { expect, type Page } from '@playwright/test'

export const baseURL = 'http://127.0.0.1:15173'
export const admin = { username: 'e2e-admin', password: ['Synthetic', 'passphrase', '2026'].join('-') }

export async function login(page: Page) {
  const response = await page.context().request.post(`${baseURL}/api/v1/auth/login`, {
    data: admin,
    headers: { Origin: baseURL },
  })
  expect(response.ok()).toBeTruthy()
  const body = await response.json() as { csrfToken: string }
  await page.addInitScript((csrfToken) => {
    window.sessionStorage.setItem('video-record.csrf-token', csrfToken)
  }, body.csrfToken)
  await page.goto('/')
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  return body.csrfToken
}

export async function seedLibrary(page: Page, csrfToken: string) {
  const response = await page.context().request.post(`${baseURL}/api/v1/data/import`, {
    headers: {
      'Idempotency-Key': 'e2e-seed-library',
      Origin: baseURL,
      'X-CSRF-Token': csrfToken,
    },
    multipart: {
      file: {
        name: 'e2e-seed.json',
        mimeType: 'application/json',
        buffer: Buffer.from(JSON.stringify(seedDocument)),
      },
    },
  })
  expect(response.ok()).toBeTruthy()
  const report = await response.json() as { importedRecords: number; failures: unknown[] }
  expect(report.importedRecords).toBe(2)
  expect(report.failures).toEqual([])
}

export async function mockTMDB(page: Page) {
  await page.route('**/api/v1/tmdb/search**', async (route) => {
    await route.fulfill({ json: { results: [] } })
  })
}

export async function expectNoHorizontalOverflow(page: Page) {
  const result = await page.evaluate(() => {
    const documentElement = document.documentElement
    const overflow = documentElement.scrollWidth - documentElement.clientWidth
    const offenders = [...document.querySelectorAll<HTMLElement>('body *')]
      .map((element) => {
        const rect = element.getBoundingClientRect()
        return {
          element: element.tagName.toLowerCase(),
          id: element.id,
          className: typeof element.className === 'string' ? element.className : '',
          left: Math.round(rect.left),
          right: Math.round(rect.right),
          width: Math.round(rect.width),
        }
      })
      .filter(({ left, right }) => left < -1 || right > documentElement.clientWidth + 1)
      .slice(0, 20)
    return { clientWidth: documentElement.clientWidth, overflow, offenders }
  })
  expect(result.overflow, JSON.stringify(result, null, 2)).toBeLessThanOrEqual(1)
}

export async function expectNoBlockingA11yViolations(page: Page) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze()
  expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([])
}

const seedDocument = {
  version: 1,
  records: [
    {
      media: {
        id: 'e2e-movie',
        mediaType: 'movie',
        externalTitle: '',
        originalTitle: '',
        releaseDate: '',
        externalOverview: '',
        posterPath: '',
        backdropPath: '',
        customTitle: '静默轨道',
        customOverview: '一部用于端到端验证的合成电影。',
        customYear: '2024',
        runtimeMinutes: 112,
        externalIds: [],
        genres: [],
        seasons: [],
      },
      state: {
        status: 'wishlist',
        version: 1,
        statusSource: 'confirmed_import',
        ratingSource: 'confirmed_import',
        noteSource: 'confirmed_import',
        shareRating: false,
        shareReview: false,
      },
      tags: [],
      events: [],
      progress: [],
    },
    {
      media: {
        id: 'e2e-series',
        mediaType: 'tv',
        externalTitle: '',
        originalTitle: '',
        releaseDate: '',
        externalOverview: '',
        posterPath: '',
        backdropPath: '',
        customTitle: '潮汐档案',
        customOverview: '一部用于分集进度验证的合成剧集。',
        customYear: '2025',
        externalIds: [],
        genres: [],
        seasons: [
          {
            id: 'e2e-season-1',
            seasonNumber: 1,
            name: '第 1 季',
            overview: '',
            posterPath: '',
            airDate: '2025-01-01',
            episodes: [
              { id: 'e2e-episode-1', episodeNumber: 1, name: '潮起', overview: '', stillPath: '', airDate: '2025-01-01', runtime: 45 },
              { id: 'e2e-episode-2', episodeNumber: 2, name: '回声', overview: '', stillPath: '', airDate: '2025-01-08', runtime: 47 },
              { id: 'e2e-episode-3', episodeNumber: 3, name: '归航', overview: '', stillPath: '', airDate: '2025-01-15', runtime: 49 },
            ],
          },
        ],
      },
      state: {
        status: 'watching',
        version: 1,
        statusSource: 'confirmed_import',
        ratingSource: 'confirmed_import',
        noteSource: 'confirmed_import',
        shareRating: false,
        shareReview: false,
      },
      tags: [],
      events: [],
      progress: [],
    },
  ],
  collections: [],
}
