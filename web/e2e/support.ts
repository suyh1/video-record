import AxeBuilder from '@axe-core/playwright'
import { expect, type Locator, type Page } from '@playwright/test'
import { inflateSync } from 'node:zlib'

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

export async function settleVisual(page: Page, root: Locator = page.locator('body')) {
  await page.evaluate(() => document.fonts.ready)
  await root.evaluate(async (element) => {
    const images = [...element.querySelectorAll<HTMLImageElement>('img')]
      .filter((image) => {
        const style = getComputedStyle(image)
        const rect = image.getBoundingClientRect()
        return style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0
      })
    await Promise.all(images.map(async (image) => {
      if (typeof image.decode === 'function') {
        try {
          await image.decode()
        } catch {
          // Failed-image scenarios settle through their explicit component state.
        }
      }
    }))
  })
  await page.evaluate(() => new Promise<void>((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
}

export async function expectNoInternalHorizontalOverflow(page: Page, selectors: string[]) {
  const result = await page.evaluate((targets) => targets.flatMap((selector) => (
    [...document.querySelectorAll<HTMLElement>(selector)].flatMap((element, index) => {
      const rect = element.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) return []
      const overflow = element.scrollWidth - element.clientWidth
      return overflow > 1 ? [{ selector, index, clientWidth: element.clientWidth, scrollWidth: element.scrollWidth }] : []
    })
  )), selectors)
  expect(result, JSON.stringify(result, null, 2)).toEqual([])
}

export async function expectPosterAspectRatios(page: Page) {
  const ratios = await page.locator('.poster-frame').evaluateAll((elements) => elements.flatMap((element) => {
    const rect = element.getBoundingClientRect()
    return rect.width > 0 && rect.height > 0 ? [rect.width / rect.height] : []
  }))
  expect(ratios.length).toBeGreaterThan(0)
  for (const ratio of ratios) expect(ratio).toBeCloseTo(2 / 3, 2)
}

export async function expectVisibleContentNotClipped(page: Page) {
  const result = await page.evaluate(() => {
    const targets = [...document.querySelectorAll<HTMLElement>([
      'h1',
      'h2',
      'h3',
      'button',
      'a.button-primary',
      'a.button-secondary',
      '.record-button',
    ].join(','))]
    return targets.flatMap((element) => {
      const rect = element.getBoundingClientRect()
      const style = getComputedStyle(element)
      if (rect.width === 0 || rect.height === 0 || style.visibility === 'hidden') return []
      const horizontal = element.scrollWidth - element.clientWidth
      const vertical = element.scrollHeight - element.clientHeight
      const horizontallyClipped = horizontal > 1 && ['clip', 'hidden'].includes(style.overflowX)
      const verticallyClipped = vertical > 1 && ['clip', 'hidden'].includes(style.overflowY)
      return horizontallyClipped || verticallyClipped ? [{
        element: element.tagName.toLowerCase(),
        text: element.textContent?.trim().slice(0, 80) ?? '',
        horizontal,
        vertical,
        overflowX: style.overflowX,
        overflowY: style.overflowY,
      }] : []
    })
  })
  expect(result, JSON.stringify(result, null, 2)).toEqual([])
}

export async function expectMobileNavigationClearance(page: Page) {
  const viewport = page.viewportSize()
  if (viewport?.width && viewport.width >= 768) return
  const result = await page.evaluate(() => {
    const main = document.querySelector<HTMLElement>('#main-content')
    const navigation = document.querySelector<HTMLElement>('.mobile-navigation')
    if (!main || !navigation) return null
    const homeContent = main.querySelector<HTMLElement>('.home-content')
    return {
      navigationHeight: navigation.getBoundingClientRect().height,
      contentReserve: Math.max(
        Number.parseFloat(getComputedStyle(main).paddingBottom),
        homeContent ? Number.parseFloat(getComputedStyle(homeContent).paddingBottom) : 0,
      ),
    }
  })
  expect(result).not.toBeNull()
  expect(result!.contentReserve + 1).toBeGreaterThanOrEqual(result!.navigationHeight)
}

export async function expectScreenshotPixels(
  page: Page,
  samples: Array<{ label: string; x: number; y: number }>,
  expected: [number, number, number] = [255, 255, 255],
) {
  const screenshot = await page.screenshot({ animations: 'disabled' })
  const png = decodePNG(screenshot)
  for (const sample of samples) {
    const x = Math.max(0, Math.min(png.width - 1, Math.round(sample.x)))
    const y = Math.max(0, Math.min(png.height - 1, Math.round(sample.y)))
    const offset = (y * png.width + x) * 4
    expect(
      [...png.pixels.subarray(offset, offset + 4)],
      `${sample.label} at ${x},${y}`,
    ).toEqual([...expected, 255])
  }
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

function decodePNG(contents: Buffer) {
  const signature = contents.subarray(0, 8)
  expect([...signature]).toEqual([137, 80, 78, 71, 13, 10, 26, 10])
  let width = 0
  let height = 0
  let bitDepth = 0
  let colorType = 0
  let interlace = 0
  const imageData: Buffer[] = []
  for (let offset = 8; offset < contents.length;) {
    const length = contents.readUInt32BE(offset)
    const type = contents.toString('ascii', offset + 4, offset + 8)
    const data = contents.subarray(offset + 8, offset + 8 + length)
    if (type === 'IHDR') {
      width = data.readUInt32BE(0)
      height = data.readUInt32BE(4)
      bitDepth = data[8] ?? 0
      colorType = data[9] ?? 0
      interlace = data[12] ?? 0
    } else if (type === 'IDAT') {
      imageData.push(data)
    }
    offset += length + 12
  }
  if (bitDepth !== 8 || ![2, 6].includes(colorType) || interlace !== 0) {
    throw new Error(`Unsupported screenshot PNG: depth=${bitDepth} color=${colorType} interlace=${interlace}`)
  }
  const bytesPerPixel = colorType === 6 ? 4 : 3
  const stride = width * bytesPerPixel
  const inflated = inflateSync(Buffer.concat(imageData))
  const unfiltered = Buffer.alloc(stride * height)
  for (let row = 0; row < height; row += 1) {
    const inputOffset = row * (stride + 1)
    const outputOffset = row * stride
    const filter = inflated[inputOffset] ?? 0
    for (let column = 0; column < stride; column += 1) {
      const raw = inflated[inputOffset + 1 + column] ?? 0
      const left = column >= bytesPerPixel ? unfiltered[outputOffset + column - bytesPerPixel] ?? 0 : 0
      const up = row > 0 ? unfiltered[outputOffset + column - stride] ?? 0 : 0
      const upperLeft = row > 0 && column >= bytesPerPixel
        ? unfiltered[outputOffset + column - stride - bytesPerPixel] ?? 0
        : 0
      const prediction = filter === 0 ? 0
        : filter === 1 ? left
          : filter === 2 ? up
            : filter === 3 ? Math.floor((left + up) / 2)
              : filter === 4 ? paeth(left, up, upperLeft)
                : Number.NaN
      if (!Number.isFinite(prediction)) throw new Error(`Unsupported PNG filter: ${filter}`)
      unfiltered[outputOffset + column] = (raw + prediction) & 0xff
    }
  }
  const pixels = Buffer.alloc(width * height * 4)
  for (let source = 0, target = 0; source < unfiltered.length; source += bytesPerPixel, target += 4) {
    pixels[target] = unfiltered[source] ?? 0
    pixels[target + 1] = unfiltered[source + 1] ?? 0
    pixels[target + 2] = unfiltered[source + 2] ?? 0
    pixels[target + 3] = colorType === 6 ? unfiltered[source + 3] ?? 0 : 255
  }
  return { height, pixels, width }
}

function paeth(left: number, up: number, upperLeft: number) {
  const prediction = left + up - upperLeft
  const leftDistance = Math.abs(prediction - left)
  const upDistance = Math.abs(prediction - up)
  const upperLeftDistance = Math.abs(prediction - upperLeft)
  if (leftDistance <= upDistance && leftDistance <= upperLeftDistance) return left
  if (upDistance <= upperLeftDistance) return up
  return upperLeft
}

const seedDocument = {
  version: 2,
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
      profile: {
        version: 1,
        shareRating: false,
        shareReview: false,
      },
      tags: [],
      rounds: [{
        id: 'e2e-movie-round-1',
        seasonNumber: null,
        roundNumber: 1,
        status: 'wishlist',
        version: 1,
        statusSource: 'confirmed_import',
        ratingSource: 'confirmed_import',
        noteSource: 'confirmed_import',
        events: [],
        episodes: [],
      }],
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
      profile: {
        version: 1,
        shareRating: false,
        shareReview: false,
      },
      tags: [],
      rounds: [{
        id: 'e2e-series-season-1-round-1',
        seasonNumber: 1,
        roundNumber: 1,
        status: 'watching',
        version: 1,
        statusSource: 'confirmed_import',
        ratingSource: 'confirmed_import',
        noteSource: 'confirmed_import',
        events: [],
        episodes: [],
      }],
    },
  ],
  collections: [],
}
