import { expect, test } from '@playwright/test'

import { expectImageLoaded, expectNoFixedElementOverlap, expectNoHorizontalOverflow, login } from './support'

test('matches the personalized light and dark responsive home views through the signed image proxy', async ({ page }) => {
  const directTMDBImages: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).hostname.endsWith('image.tmdb.org')) directTMDBImages.push(request.url())
  })
  await login(page)

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
      await expectImageLoaded(page.getByRole('img', { name: '潮汐档案 背景' }))
      await expectNoHorizontalOverflow(page)
      await expect(page).toHaveScreenshot(`details-${viewport.width}x${viewport.height}-${theme}.png`, {
        animations: 'disabled',
        fullPage: true,
        maxDiffPixelRatio: 0.01,
      })
    }
  }
})

const viewports = [
  { width: 375, height: 812 },
  { width: 768, height: 1024 },
  { width: 1440, height: 900 },
]

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
