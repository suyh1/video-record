import AxeBuilder from '@axe-core/playwright'
import { expect, type Locator, type Page } from '@playwright/test'

export const baseURL = 'http://127.0.0.1:15173'
export const syntheticTMDBOrigin = process.env.E2E_TMDB_ORIGIN ?? 'http://127.0.0.1:18082'
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

export async function controlSyntheticTMDB(page: Page, failingIds: number[] = [], resetCounts = false) {
  const response = await page.request.post(`${syntheticTMDBOrigin}/__control`, {
    data: { failingIds, resetCounts },
  })
  expect(response.ok()).toBeTruthy()
}

export async function syntheticTMDBCounts(page: Page) {
  const response = await page.request.get(`${syntheticTMDBOrigin}/__counts`)
  expect(response.ok()).toBeTruthy()
  return response.json() as Promise<Record<string, number>>
}

export async function expectImageLoaded(image: Locator) {
  await expect(image).toBeVisible()
  await expect.poll(() => image.evaluate((element) => {
    const target = element as HTMLImageElement
    return target.complete && target.naturalWidth > 0 && target.naturalHeight > 0
  })).toBeTruthy()
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

export async function expectNoFixedElementOverlap(page: Page) {
  const overlaps = await page.evaluate(() => {
    const elements = [...document.querySelectorAll<HTMLElement>('body *')]
      .filter((element) => {
        const style = getComputedStyle(element)
        const rect = element.getBoundingClientRect()
        return (style.position === 'fixed' || style.position === 'sticky') && rect.width > 0 && rect.height > 0
      })
    const collisions: string[] = []
    for (let leftIndex = 0; leftIndex < elements.length; leftIndex += 1) {
      const left = elements[leftIndex]
      if (!left) continue
      for (let rightIndex = leftIndex + 1; rightIndex < elements.length; rightIndex += 1) {
        const right = elements[rightIndex]
        if (!right || left.contains(right) || right.contains(left)) continue
        const leftRect = left.getBoundingClientRect()
        const rightRect = right.getBoundingClientRect()
        const width = Math.min(leftRect.right, rightRect.right) - Math.max(leftRect.left, rightRect.left)
        const height = Math.min(leftRect.bottom, rightRect.bottom) - Math.max(leftRect.top, rightRect.top)
        if (width > 1 && height > 1) collisions.push(`${selector(left)} <> ${selector(right)}`)
      }
    }
    return collisions

    function selector(element: HTMLElement) {
      const className = typeof element.className === 'string' && element.className
        ? `.${element.className.trim().replaceAll(/\s+/g, '.')}`
        : ''
      return `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ''}${className}`
    }
  })
  expect(overlaps, overlaps.join('\n')).toEqual([])
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
        externalIds: [{ source: 'tmdb', sourceId: '2002', mediaType: 'movie' }],
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
        externalIds: [{ source: 'tmdb', sourceId: '1001', mediaType: 'tv' }],
        genres: [],
        seasons: [],
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
