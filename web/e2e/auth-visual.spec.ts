import { expect, test, type Page } from '@playwright/test'

import {
  expectImageLoaded,
  expectNoHorizontalOverflow,
  expectNoInternalHorizontalOverflow,
  expectScreenshotPixels,
  expectVisibleContentNotClipped,
  settleVisual,
} from './support'

const imagePaths = [
  '/api/v1/public/tmdb/images/w1280/auth-one.bmp?expires=4102444800&signature=synthetic-one',
  '/api/v1/public/tmdb/images/w1280/auth-two.bmp?expires=4102444800&signature=synthetic-two',
]

const backdropOne = createAuthBackdrop(960, 540, {
  accent: [224, 146, 91],
  horizon: [47, 74, 84],
  sky: [21, 45, 66],
  water: [10, 27, 38],
})
const backdropTwo = createAuthBackdrop(960, 540, {
  accent: [204, 178, 112],
  horizon: [64, 74, 70],
  sky: [42, 39, 50],
  water: [24, 31, 38],
})

test('captures responsive TMDB-backed authentication scenes', async ({ page }) => {
  const consoleErrors = collectConsoleErrors(page)
  await installAuthRoutes(page, false)

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    for (const theme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme: theme })
      await page.goto('/')
      await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
      await waitForAuthReady(page)

      const backdrop = page.locator('.auth-backdrop .backdrop-carousel__image.is-active')
      await expectImageLoaded(backdrop)
      await expect(page.locator('.auth-page')).toHaveClass(/has-active-backdrop/)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await expect(page.getByRole('button', { name: '上一张背景' })).toBeEnabled()
      await expect(page.getByRole('button', { name: '暂停轮播' })).toBeEnabled()
      await expect(page.getByRole('button', { name: '下一张背景' })).toBeEnabled()
      await settleVisual(page)
      await expectNoHorizontalOverflow(page)
      await expectNoInternalHorizontalOverflow(page, ['.auth-panel', '.auth-form'])
      await expectVisibleContentNotClipped(page)
      await expectPanelWithinViewport(page)
      await expectControlsSeparatedFromPanel(page)

      const username = page.getByLabel('用户名')
      await username.focus()
      await expect(username).toBeFocused()

      await expect(page).toHaveScreenshot(`auth-login-${viewport.width}x${viewport.height}-${theme}-image.png`, {
        animations: 'disabled',
        fullPage: true,
        maxDiffPixels: 0,
      })

      if (theme === 'light' && viewport.width !== 768) {
        const initialSource = await backdrop.getAttribute('src')
        await page.getByRole('button', { name: '暂停轮播' }).click({ timeout: 2_000 })
        await expect(page.getByRole('button', { name: '继续轮播' })).toHaveAttribute('aria-pressed', 'true')
        await page.getByRole('button', { name: '下一张背景' }).click({ timeout: 2_000 })
        await expect(backdrop).toHaveAttribute('src', imagePaths[1]!)
        expect(await backdrop.getAttribute('src')).not.toBe(initialSource)
        await username.click()
        await expect(username).toBeFocused()
      }
    }
  }

  expect(consoleErrors).toEqual([])
})

test('keeps failed authentication backdrops white in light and dark themes', async ({ page }) => {
  const consoleErrors = collectConsoleErrors(page)
  let failedImageRequests = 0
  await installAuthRoutes(page, true, () => {
    failedImageRequests += 1
  })

  for (const viewport of viewports) {
    await page.setViewportSize(viewport)
    for (const theme of ['light', 'dark'] as const) {
      failedImageRequests = 0
      await page.goto('/')
      await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
      await waitForAuthReady(page)
      await expect.poll(() => failedImageRequests).toBeGreaterThanOrEqual(2)
      await expect(page.locator('.backdrop-carousel')).toHaveClass(/is-empty/)
      await expect(page.locator('.auth-page')).toHaveClass(/is-empty-backdrop/)
      await expect(page.locator('.auth-backdrop img')).toHaveCount(0)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await settleVisual(page)
      await expectNoHorizontalOverflow(page)
      await expectNoInternalHorizontalOverflow(page, ['.auth-panel', '.auth-form'])
      await expectVisibleContentNotClipped(page)
      await expectPanelWithinViewport(page)
      await expect(page.locator('.backdrop-carousel__controls')).toBeHidden()

      const background = await page.locator('.auth-page').evaluate((element) => getComputedStyle(element).backgroundColor)
      expect(background).toMatch(/^(?:rgb\(255, 255, 255\)|oklch\(1 0 0\))$/)
      await expectScreenshotPixels(page, [
        { label: 'top-left authentication background', x: 1, y: 1 },
        { label: 'top-right authentication background', x: viewport.width - 2, y: 1 },
        { label: 'bottom-left authentication background', x: 1, y: viewport.height - 2 },
      ])

      await expect(page).toHaveScreenshot(`auth-login-${viewport.width}x${viewport.height}-${theme}-white.png`, {
        animations: 'disabled',
        fullPage: true,
        maxDiffPixels: 0,
      })
    }
  }

  expect(consoleErrors).toEqual([])
})

test('stops authentication backdrop motion when reduced motion is requested', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await installAuthRoutes(page, false)
  await page.goto('/')
  await waitForAuthReady(page)
  await expectImageLoaded(page.locator('.auth-backdrop .backdrop-carousel__image.is-active'))

  await expect(page.getByRole('button', { name: '暂停轮播' })).toBeDisabled()
  const motion = await page.locator('.backdrop-carousel__image.is-active').evaluate((element) => {
    const style = getComputedStyle(element)
    return { animationDuration: style.animationDuration, transitionDuration: style.transitionDuration }
  })
  expect(Number.parseFloat(motion.animationDuration)).toBeLessThanOrEqual(0.001)
  expect(Number.parseFloat(motion.transitionDuration)).toBeLessThanOrEqual(0.001)
})

test('preserves password toggle focus by activation source', async ({ page }) => {
  await installAuthRoutes(page, false)
  await page.goto('/')
  await waitForAuthReady(page)

  const password = page.getByLabel('密码', { exact: true })
  await password.fill('correct horse battery staple')
  await password.focus()
  await page.keyboard.press('Tab')
  const showPassword = page.getByRole('button', { name: '显示密码' })
  await expect(showPassword).toBeFocused()

  await page.keyboard.press('Enter')
  const hidePassword = page.getByRole('button', { name: '隐藏密码' })
  await expect(hidePassword).toBeFocused()
  await expect(hidePassword).toHaveAttribute('aria-pressed', 'true')
  await expect(password).toHaveAttribute('type', 'text')
  await expect(password).toHaveValue('correct horse battery staple')

  await page.keyboard.press('Space')
  await expect(showPassword).toBeFocused()
  await expect(showPassword).toHaveAttribute('aria-pressed', 'false')
  await expect(password).toHaveAttribute('type', 'password')

  await password.evaluate((element) => {
    const input = element as HTMLInputElement
    input.focus()
    input.setSelectionRange(8, 15, 'forward')
  })
  await showPassword.click()
  await expect(password).toBeFocused()
  await expect(password).toHaveAttribute('type', 'text')
  await expect(password).toHaveValue('correct horse battery staple')
  await expect.poll(() => password.evaluate((element) => {
    const input = element as HTMLInputElement
    return { direction: input.selectionDirection, end: input.selectionEnd, start: input.selectionStart }
  })).toEqual({ direction: 'forward', end: 15, start: 8 })

  await password.evaluate((element) => {
    const input = element as HTMLInputElement
    input.setSelectionRange(8, 15, 'backward')
  })
  await hidePassword.click()
  await expect(password).toBeFocused()
  await expect(password).toHaveAttribute('type', 'password')
  await expect(password).toHaveValue('correct horse battery staple')
  await expect.poll(() => password.evaluate((element) => {
    const input = element as HTMLInputElement
    return { direction: input.selectionDirection, end: input.selectionEnd, start: input.selectionStart }
  })).toEqual({ direction: 'backward', end: 15, start: 8 })

  await password.evaluate((element) => {
    const input = element as HTMLInputElement
    input.setSelectionRange(8, 15, 'forward')
  })
  await showPassword.evaluate((button) => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
  })
  await expect(password).toHaveAttribute('type', 'password')
  await expect.poll(() => password.evaluate((element) => {
    const input = element as HTMLInputElement
    return { direction: input.selectionDirection, end: input.selectionEnd, start: input.selectionStart }
  })).toEqual({ direction: 'forward', end: 15, start: 8 })

  await password.evaluate((element) => {
    const input = element as HTMLInputElement
    input.setSelectionRange(8, 15, 'forward')
  })
  await showPassword.evaluate((button) => {
    for (let index = 0; index < 3; index += 1) {
      button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    }
  })
  await expect(password).toHaveAttribute('type', 'text')
  await expect.poll(() => password.evaluate((element) => {
    const input = element as HTMLInputElement
    return { direction: input.selectionDirection, end: input.selectionEnd, start: input.selectionStart }
  })).toEqual({ direction: 'forward', end: 15, start: 8 })
})

const viewports = [
  { width: 375, height: 812 },
  { width: 768, height: 1024 },
  { width: 1440, height: 900 },
]

async function installAuthRoutes(page: Page, failImages: boolean, onImageRequest?: () => void) {
  await page.route('**/api/v1/public/tmdb/highlights', async (route) => {
    await route.fulfill({
      body: JSON.stringify({
        items: [
          { id: 9101, mediaType: 'movie', title: '合成背景一', originalTitle: 'Synthetic One', year: '2026', overview: '', backdropURL: imagePaths[0] },
          { id: 9102, mediaType: 'tv', title: '合成背景二', originalTitle: 'Synthetic Two', year: '2026', overview: '', backdropURL: imagePaths[1] },
        ],
      }),
      contentType: 'application/json',
      status: 200,
    })
  })
  await page.route('**/api/v1/public/tmdb/images/**', async (route) => {
    onImageRequest?.()
    if (failImages) {
      await route.fulfill({ body: Buffer.from('not a decodable bitmap'), contentType: 'image/bmp', status: 200 })
      return
    }
    const body = route.request().url().includes('auth-two') ? backdropTwo : backdropOne
    await route.fulfill({ body, contentType: 'image/bmp', status: 200 })
  })
}

async function waitForAuthReady(page: Page) {
  await expect(page.getByRole('heading', { name: '登录 video-record' })).toBeVisible()
  await page.evaluate(() => document.fonts.ready)
}

async function expectPanelWithinViewport(page: Page) {
  const result = await page.evaluate(() => {
    const panel = document.querySelector<HTMLElement>('.auth-panel')?.getBoundingClientRect()
    if (!panel) return { panelFound: false }
    return {
      panelBottom: panel.bottom,
      panelFound: true,
      panelLeft: panel.left,
      panelRight: panel.right,
      panelTop: panel.top,
      viewportHeight: innerHeight,
      viewportWidth: innerWidth,
    }
  })
  expect(result, JSON.stringify(result, null, 2)).toMatchObject({ panelFound: true })
  expect(result.panelLeft).toBeGreaterThanOrEqual(0)
  expect(result.panelRight).toBeLessThanOrEqual(result.viewportWidth)
  expect(result.panelTop).toBeGreaterThanOrEqual(0)
  expect(result.panelBottom).toBeLessThanOrEqual(result.viewportHeight)
}

async function expectControlsSeparatedFromPanel(page: Page) {
  const overlap = await page.evaluate(() => {
    const panel = document.querySelector<HTMLElement>('.auth-panel')?.getBoundingClientRect()
    const controls = document.querySelector<HTMLElement>('.backdrop-carousel__controls')?.getBoundingClientRect()
    if (!panel || !controls) return true
    const overlapWidth = Math.min(panel.right, controls.right) - Math.max(panel.left, controls.left)
    const overlapHeight = Math.min(panel.bottom, controls.bottom) - Math.max(panel.top, controls.top)
    return overlapWidth > 1 && overlapHeight > 1
  })
  expect(overlap).toBe(false)
}

function collectConsoleErrors(page: Page) {
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    if (/^Failed to load resource:.*401 \(Unauthorized\)$/.test(message.text())) return
    errors.push(message.text())
  })
  page.on('response', (response) => {
    if (response.status() < 400) return
    const pathname = new URL(response.url()).pathname
    if (response.status() === 401 && pathname === '/api/v1/auth/me') return
    errors.push(`Unexpected HTTP ${response.status()} from ${pathname}`)
  })
  page.on('pageerror', (error) => errors.push(error.message))
  return errors
}

function createAuthBackdrop(width: number, height: number, colors: {
  accent: [number, number, number]
  horizon: [number, number, number]
  sky: [number, number, number]
  water: [number, number, number]
}) {
  const rowSize = Math.ceil(width * 3 / 4) * 4
  const imageSize = rowSize * height
  const buffer = Buffer.alloc(54 + imageSize)
  buffer.write('BM', 0)
  buffer.writeUInt32LE(buffer.length, 2)
  buffer.writeUInt32LE(54, 10)
  buffer.writeUInt32LE(40, 14)
  buffer.writeInt32LE(width, 18)
  buffer.writeInt32LE(height, 22)
  buffer.writeUInt16LE(1, 26)
  buffer.writeUInt16LE(24, 28)
  buffer.writeUInt32LE(imageSize, 34)

  for (let row = 0; row < height; row += 1) {
    const y = 1 - row / Math.max(height - 1, 1)
    for (let x = 0; x < width; x += 1) {
      const nx = x / Math.max(width - 1, 1)
      const mountainLine = 0.48 + Math.sin(nx * 8.2) * 0.035 + Math.sin(nx * 19.7 + 0.8) * 0.018
      const sun = Math.max(0, 1 - Math.hypot(nx - 0.72, y - 0.32) * 4.4)
      const ripple = (Math.sin(nx * 68 + y * 26) + 1) * 0.018
      let color = mix(colors.sky, colors.horizon, Math.min(1, y / 0.62))
      if (y > mountainLine) color = mix(colors.horizon, colors.water, Math.min(1, (y - mountainLine) / 0.22))
      color = color.map((value, index) => value + colors.accent[index]! * sun * 0.58 + 255 * ripple) as [number, number, number]
      const offset = 54 + row * rowSize + x * 3
      buffer[offset] = clamp(color[2])
      buffer[offset + 1] = clamp(color[1])
      buffer[offset + 2] = clamp(color[0])
    }
  }
  return buffer
}

function mix(from: [number, number, number], to: [number, number, number], amount: number) {
  return from.map((value, index) => value + (to[index]! - value) * amount) as [number, number, number]
}

function clamp(value: number) {
  return Math.max(0, Math.min(255, Math.round(value)))
}
