import { expect, test } from '@playwright/test'
import { deflateSync } from 'node:zlib'

import {
  expectImageLoaded,
  expectNoBlockingA11yViolations,
  expectNoFixedElementOverlap,
  expectNoHorizontalOverflow,
  login,
} from './support'

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
})

test('matches the personalized light and dark responsive home views through the signed image proxy', async ({ page }) => {
  const directTMDBImages: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).hostname.endsWith('image.tmdb.org')) directTMDBImages.push(request.url())
  })
  const brightBackdropRoute = '**/api/v1/public/tmdb/images/w1280/tide-backdrop.png**'
  await page.route(brightBackdropRoute, (route) => route.fulfill({
    body: createSolidPNG(16, 9, [255, 255, 255]),
    contentType: 'image/png',
    status: 200,
  }))
  await page.setViewportSize({ width: 1440, height: 900 })
  expect(await page.evaluate(() => matchMedia('(prefers-reduced-motion: reduce)').matches)).toBe(true)
  await login(page)
  await page.goto('/')
  await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'light'))
  await expect(page.getByRole('region', { name: '首页主视觉' })).toHaveAttribute('data-backdrop-state', 'ready')
  const brightBackdrop = page.locator('.home-hero .backdrop-carousel__image.is-active')
  await expectImageLoaded(brightBackdrop)
  const stableSource = await brightBackdrop.getAttribute('src')
  await page.waitForTimeout(8_250)
  await expect(brightBackdrop).toHaveAttribute('src', stableSource ?? '')
  await expectReadableImageHeader(page)
  await page.unroute(brightBackdropRoute)

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    for (const theme of ['light', 'dark'] as const) {
      await page.goto('/')
      await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
      const hero = page.getByRole('region', { name: '首页主视觉' })
      await expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
      const backdrop = hero.locator('.backdrop-carousel__image.is-active')
      await expectImageLoaded(backdrop)
      await expect(backdrop).toHaveAttribute('src', /^\/api\/v1\/public\/tmdb\/images\/w1280\//)
      await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
      await expect(page.locator('.app-header')).not.toHaveClass(/home-white-header/)
      await expect(page.locator('.app-header')).toHaveClass(/home-image-header/)
      await expectNoHorizontalOverflow(page)
      await expectContinueWatchingHint(page, viewport.height)
      await expect(page).toHaveScreenshot(`home-${viewport.width}x${viewport.height}-${theme}.png`, {
        animations: 'disabled',
        fullPage: true,
        maxDiffPixelRatio: 0.01,
      })
    }
  }

  expect(directTMDBImages).toEqual([])
})

test('keeps an all-failed mobile home hero pure white and usable in both themes', async ({ page }) => {
  await page.route('**/api/v1/public/tmdb/images/w1280/**', (route) => route.fulfill({
    body: Buffer.from('not a decodable image'),
    contentType: 'image/png',
    status: 200,
  }))
  await login(page)
  await page.setViewportSize({ width: 375, height: 812 })

  for (const theme of ['light', 'dark'] as const) {
    await page.goto('/')
    await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
    const hero = page.getByRole('region', { name: '首页主视觉' })
    await expect(hero).toHaveAttribute('data-backdrop-state', 'empty')
    await expect(hero.locator('img')).toHaveCount(0)
    await expect(page.locator('.app-header')).toHaveClass(/home-white-header/)
    await expectPureWhiteReadableHero(page)
    await expectNoHorizontalOverflow(page)
    await expectNoFixedElementOverlap(page)
    await expectContinueWatchingHint(page, 812)
    await expect(page.getByRole('button', { name: /推进 潮汐档案 下一集/ })).toBeEnabled()
    await expect(page.getByRole('link', { name: '查看 静默轨道 记录' })).toHaveAttribute('href', '/media/e2e-movie')
  }

  await expect(page).toHaveScreenshot('home-375x812-white.png', {
    animations: 'disabled',
    fullPage: true,
    maxDiffPixelRatio: 0.01,
  })
})

test('matches the approved light and dark responsive library views', async ({ page }) => {
  await login(page)

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    for (const theme of ['light', 'dark'] as const) {
      await page.goto('/library')
      await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
      await expect(page.getByRole('heading', { level: 1, name: '影库' })).toBeVisible()
      await expect(page.getByText('2 部影视')).toBeVisible()
      await expectNoHorizontalOverflow(page)
      await expect(page).toHaveScreenshot(`library-${viewport.width}x${viewport.height}-${theme}.png`, {
        animations: 'disabled',
        fullPage: true,
        maxDiffPixelRatio: 0.01,
      })
    }
  }
})

test('matches the approved light and dark responsive details views', async ({ page }) => {
  const directTMDBImages: string[] = []
  const runtimeErrors: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).hostname.endsWith('image.tmdb.org')) directTMDBImages.push(request.url())
  })
  page.on('console', (message) => {
    if (message.type() === 'error') runtimeErrors.push(message.text())
  })
  page.on('pageerror', (error) => runtimeErrors.push(error.message))
  await page.route('**/api/v1/public/tmdb/highlights', (route) => route.fulfill({
    json: { items: [] },
    status: 200,
  }))
  await login(page)

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    for (const theme of ['light', 'dark'] as const) {
      await page.goto('/media/e2e-series')
      await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
      await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
      await expect(page.getByText('林见川')).toBeVisible()
      await page.getByRole('combobox', { name: '选择季' }).selectOption('2')
      await expect(page.getByText('重返北堤')).toBeVisible()
      const hero = page.locator('.media-hero')
      const backdrop = hero.locator('.media-hero-backdrop')
      await expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
      await expectImageLoaded(backdrop)
      await expect(backdrop).toHaveAttribute('alt', '')
      await expect(backdrop).toHaveAttribute('src', /^\/api\/v1\/public\/tmdb\/images\/w1280\//)
      await expectDetailsHeaderLayout(page)
      const seasonSelector = page.getByRole('combobox', { name: '选择季' })
      await seasonSelector.focus()
      await expect(seasonSelector).toBeFocused()
      await page.keyboard.press('Tab')
      await expect(page.getByRole('button', { name: /推进下一集/ })).toBeFocused()
      await expectNoHorizontalOverflow(page)
      await expectNoFixedElementOverlap(page)
      await page.evaluate(() => {
        if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
        window.scrollTo(0, 0)
      })
      await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0)
      await expect(page).toHaveScreenshot(`details-${viewport.width}x${viewport.height}-${theme}.png`, {
        animations: 'disabled',
        fullPage: true,
        maxDiffPixelRatio: 0.01,
      })
    }
  }

  await expectNoBlockingA11yViolations(page)
  expect(directTMDBImages).toEqual([])
  expect(runtimeErrors).toEqual([])
})

test('keeps failed details imagery and a long title neutral, named, and usable', async ({ page }) => {
  const directTMDBImages: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).hostname.endsWith('image.tmdb.org')) directTMDBImages.push(request.url())
  })
  await page.route('**/api/v1/public/tmdb/highlights', (route) => route.fulfill({
    json: { items: [] },
    status: 200,
  }))
  await login(page)
  await page.route('**/api/v1/media/e2e-series', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as Record<string, unknown>
    await route.fulfill({
      response,
      json: { ...body, title: '潮汐档案：在漫长海岸线上追索一段被遗忘的家庭影像记录' },
    })
  })
  await page.route('**/api/v1/public/tmdb/images/w1280/tide-backdrop.png**', (route) => route.fulfill({
    body: Buffer.from('not a decodable image'),
    contentType: 'image/png',
    status: 200,
  }))
  await page.route('**/api/v1/public/tmdb/images/w300/cast-one.png**', (route) => route.fulfill({
    body: Buffer.from('not a decodable image'),
    contentType: 'image/png',
    status: 200,
  }))
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto('/media/e2e-series')

  await expect(page.getByRole('heading', {
    level: 1,
    name: '潮汐档案：在漫长海岸线上追索一段被遗忘的家庭影像记录',
  })).toBeVisible()
  const hero = page.locator('.media-hero')
  await expect(hero).toHaveAttribute('data-backdrop-state', 'failed')
  await expect(hero.locator('.media-hero-backdrop')).toHaveCount(0)
  await expect(page.getByRole('img', { name: '林见川 饰 顾潮 暂无头像' })).toBeVisible()
  await expectDetailsHeaderLayout(page)

  const seasonSelector = page.getByRole('combobox', { name: '选择季' })
  await seasonSelector.selectOption('2')
  await seasonSelector.focus()
  await page.keyboard.press('Tab')
  await expect(page.getByRole('button', { name: /推进下一集/ })).toBeFocused()
  await expectNoHorizontalOverflow(page)
  await expectNoFixedElementOverlap(page)
  await expectNoBlockingA11yViolations(page)
  expect(directTMDBImages).toEqual([])
})

test('keeps the details header readable throughout normal-motion backdrop readiness', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.route('**/api/v1/public/tmdb/highlights', (route) => route.fulfill({
    json: { items: [] },
    status: 200,
  }))
  await login(page)

  const contrastBackdrop = `/api/v1/public/tmdb/images/w1280/contrast-backdrop.png?expires=4102444800&signature=${'d'.repeat(64)}`
  await page.route('**/api/v1/tmdb/tv/1001', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as Record<string, unknown>
    await route.fulfill({ response, json: { ...body, backdropPath: contrastBackdrop } })
  })
  let markBackdropRequested!: () => void
  let releaseBackdrop!: () => void
  const backdropRequested = new Promise<void>((resolve) => { markBackdropRequested = resolve })
  const backdropHeld = new Promise<void>((resolve) => { releaseBackdrop = resolve })
  await page.route('**/api/v1/public/tmdb/images/w1280/contrast-backdrop.png**', async (route) => {
    markBackdropRequested()
    await backdropHeld
    await route.fulfill({
      body: createSolidPNG(16, 9, [255, 255, 255]),
      contentType: 'image/png',
      status: 200,
    })
  })

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/media/e2e-series', { waitUntil: 'domcontentloaded' })
  await backdropRequested
  const hero = page.locator('.media-hero')
  await expect(hero).toHaveAttribute('data-backdrop-state', 'loading')
  await installDetailsHeaderTransitionSampler(page)
  releaseBackdrop()
  const samples = await readDetailsHeaderTransitionSamples(page)

  await expect(hero).toHaveAttribute('data-backdrop-state', 'ready')
  expect(samples.length).toBeGreaterThanOrEqual(21)
  for (const sample of samples) {
    for (const entry of sample.entries) {
      expect(
        minimumCompositedContrast(sample.headerBackground, entry.background, entry.foreground),
        `${sample.phase} ${entry.name}: ${entry.foreground} on ${entry.background} / ${sample.headerBackground}`,
      ).toBeGreaterThanOrEqual(4.5)
    }
  }

  const transitionDurations = await page.evaluate(() => [
    getComputedStyle(document.querySelector<HTMLElement>('.app-header')!).transitionDuration,
    getComputedStyle(document.querySelector<HTMLElement>('.app-primary-navigation .nav-link')!).transitionDuration,
    getComputedStyle(document.querySelector<HTMLElement>('.global-search')!).transitionDuration,
  ])
  for (const duration of transitionDurations.flatMap((value) => value.split(', '))) {
    expect(Number.parseFloat(duration)).toBeLessThanOrEqual(0.001)
  }
})

const viewports = [
  { width: 375, height: 812 },
  { width: 768, height: 1024 },
  { width: 1440, height: 900 },
]

type HeaderTransitionSample = {
  phase: string
  headerBackground: string
  entries: Array<{ name: string; background: string; foreground: string }>
}

function installDetailsHeaderTransitionSampler(page: import('@playwright/test').Page) {
  return page.evaluate(() => {
    const target = globalThis as typeof globalThis & {
      __detailsHeaderTransitionSamples?: Promise<HeaderTransitionSample[]>
    }
    const hero = document.querySelector<HTMLElement>('.media-hero')!
    const header = document.querySelector<HTMLElement>('.app-header')!
    target.__detailsHeaderTransitionSamples = new Promise<HeaderTransitionSample[]>((resolve) => {
      const samples: HeaderTransitionSample[] = []
      let readyFrames = 0

      const capture = (phase: string) => {
        const navEntries = [...document.querySelectorAll<HTMLElement>('.app-primary-navigation .nav-link')]
          .map((element) => ({
            name: `nav:${element.textContent?.trim() ?? ''}`,
            background: getComputedStyle(element).backgroundColor,
            foreground: getComputedStyle(element).color,
          }))
        const search = document.querySelector<HTMLElement>('.global-search')!
        const searchInput = search.querySelector<HTMLInputElement>('input')!
        samples.push({
          phase,
          headerBackground: getComputedStyle(header).backgroundColor,
          entries: [
            {
              name: 'brand',
              background: getComputedStyle(document.querySelector<HTMLElement>('.brand')!).backgroundColor,
              foreground: getComputedStyle(document.querySelector<HTMLElement>('.brand')!).color,
            },
            ...navEntries,
            {
              name: 'search-placeholder',
              background: getComputedStyle(search).backgroundColor,
              foreground: getComputedStyle(searchInput, '::placeholder').color,
            },
          ],
        })
      }
      const captureFrame = () => {
        capture(`rAF-${readyFrames}`)
        readyFrames += 1
        if (readyFrames >= 20) resolve(samples)
        else requestAnimationFrame(captureFrame)
      }
      const observer = new MutationObserver(() => {
        if (!hero.classList.contains('has-backdrop')) return
        observer.disconnect()
        capture('ready-mutation')
        requestAnimationFrame(captureFrame)
      })
      observer.observe(hero, { attributeFilter: ['class'], attributes: true })
    })
  })
}

function readDetailsHeaderTransitionSamples(page: import('@playwright/test').Page) {
  return page.evaluate(() => {
    const target = globalThis as typeof globalThis & {
      __detailsHeaderTransitionSamples?: Promise<HeaderTransitionSample[]>
    }
    if (!target.__detailsHeaderTransitionSamples) throw new Error('Header transition sampler was not installed')
    return target.__detailsHeaderTransitionSamples
  })
}

async function expectDetailsHeaderLayout(page: import('@playwright/test').Page) {
  const header = page.getByRole('banner', { name: '应用导航' })
  const hero = page.locator('.media-hero')
  const heroContent = hero.locator('.media-hero-content')
  const castHeading = page.getByRole('heading', { level: 2, name: '主要演员' })
  const skipLink = page.getByRole('link', { name: '跳到主要内容' })

  expect(await page.evaluate(() => window.scrollY)).toBe(0)
  const [headerBox, heroBox, contentBox, castBox, skipBox] = await Promise.all([
    header.boundingBox(),
    hero.boundingBox(),
    heroContent.boundingBox(),
    castHeading.boundingBox(),
    skipLink.boundingBox(),
  ])
  expect(headerBox?.y).toBe(0)
  expect(heroBox?.y).toBe(0)
  expect(contentBox?.y).toBeGreaterThanOrEqual(headerBox?.height ?? 0)
  expect(castBox?.y).toBeGreaterThanOrEqual((heroBox?.y ?? 0) + (heroBox?.height ?? 0))
  expect((skipBox?.y ?? 0) + (skipBox?.height ?? 0)).toBeLessThanOrEqual(0)

  await page.evaluate(() => window.scrollTo(0, 96))
  await expect(header).toHaveClass(/is-scrolled/)
  expect((await header.boundingBox())?.y).toBe(0)
  expect(await header.evaluate((element) => getComputedStyle(element).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)')
  await page.evaluate(() => window.scrollTo(0, 0))
  await expect(header).not.toHaveClass(/is-scrolled/)
}

async function expectContinueWatchingHint(page: import('@playwright/test').Page, viewportHeight: number) {
  const section = page.getByRole('region', { name: '继续观看' })
  await expect(section).toBeVisible()
  const top = await section.evaluate((element) => element.getBoundingClientRect().top)
  expect(top).toBeLessThan(viewportHeight)
}

async function expectPureWhiteReadableHero(page: import('@playwright/test').Page) {
  const colors = await page.evaluate(() => {
    const hero = document.querySelector<HTMLElement>('.home-hero')!
    const brand = document.querySelector<HTMLElement>('.app-header .brand')!
    return {
      background: getComputedStyle(hero).backgroundColor,
      foreground: getComputedStyle(hero).color,
      headerForeground: getComputedStyle(brand).color,
    }
  })
  expect(luminance(colors.background)).toBeCloseTo(1, 5)
  expect(contrastRatio(colors.background, colors.foreground)).toBeGreaterThanOrEqual(4.5)
  expect(contrastRatio(colors.background, colors.headerForeground)).toBeGreaterThanOrEqual(4.5)
}

async function expectReadableImageHeader(page: import('@playwright/test').Page) {
  const colors = await page.evaluate(() => {
    const header = document.querySelector<HTMLElement>('.app-header')!
    const headerBackground = getComputedStyle(header).backgroundColor
    const entries = [
      ['brand', document.querySelector<HTMLElement>('.app-header .brand')!, null],
      ...[...document.querySelectorAll<HTMLElement>('.app-primary-navigation .nav-link')]
        .map((element) => [`nav:${element.textContent?.trim() ?? ''}`, element, null]),
      ['search', document.querySelector<HTMLElement>('.global-search')!, '::placeholder'],
      ['record', document.querySelector<HTMLElement>('.app-header .record-button')!, null],
    ] as Array<[string, HTMLElement, string | null]>
    return entries.map(([name, element, pseudo]) => {
      const foregroundElement = pseudo ? element.querySelector<HTMLElement>('input')! : element
      return {
        background: getComputedStyle(element).backgroundColor,
        foreground: getComputedStyle(foregroundElement, pseudo).color,
        headerBackground,
        name,
      }
    })
  })

  for (const entry of colors) {
    const background = colorAlpha(entry.background) === 0 ? entry.headerBackground : entry.background
    expect(colorAlpha(background), `${entry.name} background: ${background}`).toBe(1)
    expect(
      contrastRatio(background, entry.foreground),
      `${entry.name}: ${entry.foreground} on ${background}`,
    ).toBeGreaterThanOrEqual(4.5)
  }
}

function contrastRatio(background: string, foreground: string) {
  const backgroundLuminance = luminance(background)
  const foregroundLuminance = luminance(foreground)
  const lighter = Math.max(backgroundLuminance, foregroundLuminance)
  const darker = Math.min(backgroundLuminance, foregroundLuminance)
  return (lighter + 0.05) / (darker + 0.05)
}

function luminance(color: string) {
  if (color.startsWith('rgb')) {
    const channels = color.match(/[\d.]+/g)?.slice(0, 3).map(Number) ?? []
    if (channels.length !== 3) throw new Error(`Unsupported computed color: ${color}`)
    return channels.reduce((sum, channel, index) => {
      const normalized = channel! / 255
      const linear = normalized <= 0.04045 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4
      return sum + linear * [0.2126, 0.7152, 0.0722][index]!
    }, 0)
  }

  const match = color.match(/^oklch\(([\d.]+)(%)?\s+([\d.]+)\s+([\d.]+)/)
  if (!match) throw new Error(`Unsupported computed color: ${color}`)
  const lightness = Number(match[1]) / (match[2] ? 100 : 1)
  const chroma = Number(match[3])
  const hue = Number(match[4]) * Math.PI / 180
  const a = chroma * Math.cos(hue)
  const b = chroma * Math.sin(hue)
  const l = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3
  const red = clampLinear(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s)
  const green = clampLinear(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s)
  const blue = clampLinear(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s)
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue
}

function clampLinear(value: number) {
  return Math.min(1, Math.max(0, value))
}

function colorAlpha(color: string) {
  if (color === 'transparent') return 0
  if (color.startsWith('rgb')) {
    const channels = color.match(/[\d.]+/g) ?? []
    return channels.length > 3 ? Number(channels[3]) : 1
  }
  const alpha = color.match(/\/\s*([\d.]+)(%)?\s*\)$/)
  if (!alpha) return 1
  return Number(alpha[1]) / (alpha[2] ? 100 : 1)
}

type RGBA = { red: number; green: number; blue: number; alpha: number }

function minimumCompositedContrast(headerColor: string, elementColor: string, foregroundColor: string) {
  const header = parseRGBA(headerColor)
  const element = parseRGBA(elementColor)
  const foreground = parseRGBA(foregroundColor)
  return Math.min(...[
    { red: 0, green: 0, blue: 0, alpha: 1 },
    { red: 1, green: 1, blue: 1, alpha: 1 },
  ].map((underlay) => {
    const headerBackground = compositeRGBA(header, underlay)
    const elementBackground = compositeRGBA(element, headerBackground)
    const visibleForeground = compositeRGBA(foreground, elementBackground)
    const backgroundLuminance = rgbaLuminance(elementBackground)
    const foregroundLuminance = rgbaLuminance(visibleForeground)
    const lighter = Math.max(backgroundLuminance, foregroundLuminance)
    const darker = Math.min(backgroundLuminance, foregroundLuminance)
    return (lighter + 0.05) / (darker + 0.05)
  }))
}

function parseRGBA(color: string): RGBA {
  if (color === 'transparent') return { red: 0, green: 0, blue: 0, alpha: 0 }
  if (color.startsWith('rgb')) {
    const channels = color.match(/[\d.]+/g)?.map(Number) ?? []
    if (channels.length < 3) throw new Error(`Unsupported computed color: ${color}`)
    return {
      red: channels[0]! / 255,
      green: channels[1]! / 255,
      blue: channels[2]! / 255,
      alpha: channels[3] ?? 1,
    }
  }

  const labMatch = color.match(/^oklab\(([-\d.]+)(%)?\s+([-\d.]+)\s+([-\d.]+)(?:\s*\/\s*([\d.]+)(%)?)?\)$/)
  if (labMatch) {
    return oklabToRGBA(
      Number(labMatch[1]) / (labMatch[2] ? 100 : 1),
      Number(labMatch[3]),
      Number(labMatch[4]),
      labMatch[5] ? Number(labMatch[5]) / (labMatch[6] ? 100 : 1) : 1,
    )
  }

  const match = color.match(/^oklch\(([\d.]+)(%)?\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+)(%)?)?\)$/)
  if (!match) throw new Error(`Unsupported computed color: ${color}`)
  const lightness = Number(match[1]) / (match[2] ? 100 : 1)
  const chroma = Number(match[3])
  const hue = Number(match[4]) * Math.PI / 180
  return oklabToRGBA(
    lightness,
    chroma * Math.cos(hue),
    chroma * Math.sin(hue),
    match[5] ? Number(match[5]) / (match[6] ? 100 : 1) : 1,
  )
}

function oklabToRGBA(lightness: number, a: number, b: number, alpha: number): RGBA {
  const l = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const m = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3
  return {
    red: linearToSRGB(clampLinear(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s)),
    green: linearToSRGB(clampLinear(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s)),
    blue: linearToSRGB(clampLinear(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s)),
    alpha,
  }
}

function compositeRGBA(foreground: RGBA, background: RGBA): RGBA {
  const alpha = foreground.alpha + background.alpha * (1 - foreground.alpha)
  if (alpha === 0) return { red: 0, green: 0, blue: 0, alpha: 0 }
  return {
    red: (foreground.red * foreground.alpha + background.red * background.alpha * (1 - foreground.alpha)) / alpha,
    green: (foreground.green * foreground.alpha + background.green * background.alpha * (1 - foreground.alpha)) / alpha,
    blue: (foreground.blue * foreground.alpha + background.blue * background.alpha * (1 - foreground.alpha)) / alpha,
    alpha,
  }
}

function rgbaLuminance(color: RGBA) {
  return 0.2126 * srgbToLinear(color.red)
    + 0.7152 * srgbToLinear(color.green)
    + 0.0722 * srgbToLinear(color.blue)
}

function srgbToLinear(value: number) {
  return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
}

function linearToSRGB(value: number) {
  return value <= 0.0031308 ? value * 12.92 : 1.055 * value ** (1 / 2.4) - 0.055
}

function createSolidPNG(width: number, height: number, color: [number, number, number]) {
  const rowSize = 1 + width * 3
  const pixels = Buffer.alloc(rowSize * height)
  for (let row = 0; row < height; row += 1) {
    pixels[row * rowSize] = 0
    for (let column = 0; column < width; column += 1) {
      const offset = row * rowSize + 1 + column * 3
      pixels[offset] = color[0]
      pixels[offset + 1] = color[1]
      pixels[offset + 2] = color[2]
    }
  }
  const header = Buffer.alloc(13)
  header.writeUInt32BE(width, 0)
  header.writeUInt32BE(height, 4)
  header[8] = 8
  header[9] = 2
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    pngChunk('IHDR', header),
    pngChunk('IDAT', deflateSync(pixels)),
    pngChunk('IEND', Buffer.alloc(0)),
  ])
}

function pngChunk(type: string, contents: Buffer) {
  const typeBytes = Buffer.from(type)
  const chunk = Buffer.alloc(12 + contents.length)
  chunk.writeUInt32BE(contents.length, 0)
  typeBytes.copy(chunk, 4)
  contents.copy(chunk, 8)
  chunk.writeUInt32BE(crc32(Buffer.concat([typeBytes, contents])), 8 + contents.length)
  return chunk
}

function crc32(contents: Buffer) {
  let crc = 0xffffffff
  for (const byte of contents) {
    crc ^= byte
    for (let bit = 0; bit < 8; bit += 1) crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1))
  }
  return (crc ^ 0xffffffff) >>> 0
}
