import { expect, test, type Locator, type Page, type Route } from '@playwright/test'

import {
  baseURL,
  expectNoBlockingA11yViolations,
  expectNoFixedElementOverlap,
  expectNoHorizontalOverflow,
  login,
} from './support'

test('has no blocking WCAG 2.2 AA violations on major pages', async ({ page }) => {
  await login(page)
  for (const path of ['/', '/library', '/calendar', '/stats', '/settings', '/settings/sync', '/media/e2e-series', '/missing-archive']) {
    await page.goto(path)
    await expect(page.locator('main')).toBeVisible()
    if (path === '/media/e2e-series') await expect(page.getByText('低潮线')).toBeVisible()
    await expectNoBlockingA11yViolations(page)
  }
})

test('keeps ordinary and immersive navigation stable across responsive viewports', async ({ page }) => {
  await login(page)

  for (const viewport of detailViewports) {
    await page.setViewportSize(viewport)

    await page.goto('/library')
    const ordinaryHeader = page.getByRole('banner', { name: '应用导航' })
    const primaryNavigation = page.getByRole('navigation', { name: '主导航' })
    const mobileNavigation = page.getByRole('navigation', { name: '移动导航' })
    await expect(ordinaryHeader).toHaveClass(/solid-header/)
    await expect(ordinaryHeader).not.toHaveClass(/immersive-header/)
    await expect(ordinaryHeader).toHaveCSS('height', viewport.width < 768 ? '56px' : '64px')
    const ordinaryLayout = await page.locator('#main-content').evaluate((main) => {
      const header = document.querySelector<HTMLElement>('.app-header')
      if (!header) throw new Error('Application header is missing')
      return {
        headerBottom: header.getBoundingClientRect().bottom,
        mainTop: main.getBoundingClientRect().top,
      }
    })
    expect(ordinaryLayout.mainTop).toBeGreaterThanOrEqual(ordinaryLayout.headerBottom)

    if (viewport.width < 768) {
      await expect(primaryNavigation).toBeHidden()
      await expect(mobileNavigation).toBeVisible()
      await expect(page.getByRole('link', { name: 'video-record 首页' })).toBeVisible()
      await expect(ordinaryHeader.getByRole('button', { name: '记录', exact: true })).toBeVisible()
      const clearance = await page.locator('#main-content').evaluate((main) => {
        const mobile = document.querySelector<HTMLElement>('.mobile-navigation')
        if (!mobile) throw new Error('Mobile navigation is missing')
        return {
          navigationHeight: mobile.getBoundingClientRect().height,
          paddingBottom: Number.parseFloat(getComputedStyle(main).paddingBottom),
        }
      })
      expect(clearance.paddingBottom).toBeGreaterThanOrEqual(clearance.navigationHeight)
    } else {
      await expect(primaryNavigation).toBeVisible()
      await expect(mobileNavigation).toBeHidden()
      for (const name of ['首页', '影库', '日历', '统计', '设置']) {
        await expect(primaryNavigation.getByRole('link', { name, exact: true })).toBeVisible()
      }
    }
    await expectNoHorizontalOverflow(page)
    await expectNoFixedElementOverlap(page)

    await page.goto('/')
    const immersiveHeader = page.getByRole('banner', { name: '应用导航' })
    await expect(immersiveHeader).toHaveClass(/immersive-header/)
    await expect(immersiveHeader).not.toHaveClass(/is-scrolled/)
    const immersiveLayout = await page.locator('#main-content').evaluate((main) => {
      const header = document.querySelector<HTMLElement>('.app-header')
      if (!header) throw new Error('Application header is missing')
      return {
        headerBottom: header.getBoundingClientRect().bottom,
        mainTop: main.getBoundingClientRect().top,
      }
    })
    expect(immersiveLayout.mainTop).toBeLessThan(immersiveLayout.headerBottom)
    await page.evaluate(() => window.scrollTo(0, 40))
    await expect(immersiveHeader).toHaveClass(/is-scrolled/)
    await expectNoHorizontalOverflow(page)
    await expectNoFixedElementOverlap(page)
  }
})

test('keeps real loading skeletons visible in light and dark themes', async ({ page }) => {
  await login(page)
  const requestHold = await holdRequest(page, '**/api/v1/library*', 'GET')

  try {
    await page.goto('/library')
    const skeleton = page.locator('.library-poster-skeleton').first()
    await expect(skeleton).toBeVisible()

    for (const theme of ['light', 'dark'] as const) {
      await page.evaluate((selectedTheme) => document.documentElement.setAttribute('data-theme', selectedTheme), theme)
      const contrast = await computedBackgroundContrast(skeleton)
      expect(contrast, `${theme} skeleton contrast`).toBeGreaterThanOrEqual(1.25)
    }
  } finally {
    await requestHold.release()
  }
})

test('keeps busy visuals separate from native disabled semantics', async ({ page }) => {
  await login(page)
  await page.evaluate(() => {
    const button = document.createElement('button')
    button.id = 'busy-foundation-probe'
    button.className = 'button-primary'
    button.type = 'button'
    button.ariaBusy = 'true'
    button.dataset.activations = '0'
    button.textContent = '正在处理'
    button.addEventListener('click', () => {
      button.dataset.activations = String(Number(button.dataset.activations) + 1)
    })
    document.body.append(button)
  })

  const busyButton = page.locator('#busy-foundation-probe')
  await expect(busyButton).toBeVisible()
  expect(await busyButton.evaluate((element) => getComputedStyle(element).pointerEvents)).not.toBe('none')
  await busyButton.focus()
  await page.keyboard.press('Enter')
  await expect(busyButton).toHaveAttribute('data-activations', '1')
})

test('keeps specialized form controls at the shared minimum height', async ({ page }) => {
  await login(page)

  await page.goto('/media/e2e-series')
  await expectControlMinHeight(page.getByRole('combobox', { name: '选择季' }))

  await page.goto('/library')
  await expectControlMinHeight(page.getByLabel('片单名称'))

  await page.goto('/settings')
  await expectControlMinHeight(page.getByLabel('服务类型'))
  await expectControlMinHeight(page.getByLabel('账户名称'))
  await page.getByRole('button', { name: '添加成员' }).click()
  await expectControlMinHeight(page.locator('.member-create-form').getByLabel('用户名'))
})

test('keeps a focused invalid record input on the semantic error border', async ({ page }) => {
  await login(page)
  await page.goto('/media/e2e-movie')

  await page.getByRole('radio', { name: '看过' }).click()
  const watchedAt = page.getByLabel('完成观看时间')
  await watchedAt.fill('2099-01-01T00:00:01')
  await page.getByRole('button', { name: '保存记录' }).click()

  await expect(watchedAt).toHaveAttribute('aria-invalid', 'true')
  await expect(watchedAt).toBeFocused()
  await expectTokenStyle(watchedAt, 'borderTopColor', '--error')
})

test('keeps a disabled specialized input on the disabled surface', async ({ page }) => {
  await login(page)
  await page.goto('/media/e2e-series')
  await page.getByRole('combobox', { name: '选择季' }).selectOption('2')
  await expect(page.getByText('重返北堤')).toBeVisible()

  const requestHold = await holdRequest(page, '**/api/v1/records/e2e-series/progress?seasonNumber=2', 'POST')

  const timeButton = page.getByRole('button', { name: '设置 S02E01 观看时间' })
  await timeButton.click()
  const timeInput = page.getByRole('textbox', { name: 'S02E01 观看时间' })
  await timeInput.fill('2026-07-13T21:22:23')
  await timeInput.press('Enter')

  try {
    await expect(timeInput).toBeDisabled()
    const confirmButton = page.getByRole('button', { name: '确定 S02E01 观看时间' })
    await expect(confirmButton).toBeDisabled()
    await confirmButton.focus()
    await expect(confirmButton).not.toBeFocused()
    await page.keyboard.press('Enter')
    expect(requestHold.requestCount()).toBe(1)
    await expectTokenStyle(timeInput, 'backgroundColor', '--surface')
    await expectTokenStyle(timeInput, 'color', '--muted')
    await expect.poll(() => timeInput.evaluate((element) => getComputedStyle(element).cursor)).toBe('not-allowed')
  } finally {
    await requestHold.release()
  }
})

test('keeps movie and season archive dialogs accessible and within the viewport', async ({ page }) => {
  await login(page)

  const movieRoundResponse = await page.request.get(`${baseURL}/api/v1/records/e2e-movie/rounds/current`)
  const movieRound = await movieRoundResponse.json() as { roundNumber: number }
  await page.goto('/media/e2e-movie')
  await page.getByRole('radio', { name: '看过' }).click()
  await page.getByLabel('完成观看时间').fill('2026-07-13T20:30:45')
  await page.getByRole('button', { name: '保存记录' }).click()
  await expect(page.getByRole('status')).toContainText('记录已保存')
  await page.getByRole('button', { name: '再刷' }).click()
  await page.getByRole('button', { name: `查看第 ${movieRound.roundNumber} 刷` }).click()
  const movieDialog = page.getByRole('dialog', { name: `第 ${movieRound.roundNumber} 刷记录` })
  await expect(movieDialog).toBeVisible()
  await expectDialogWithinViewport(movieDialog)
  await expectNoBlockingA11yViolations(page)
  await page.keyboard.press('Escape')

  await page.setViewportSize({ width: 375, height: 812 })
  const seasonRoundResponse = await page.request.get(`${baseURL}/api/v1/records/e2e-series/rounds/current?seasonNumber=1`)
  const seasonRound = await seasonRoundResponse.json() as { roundNumber: number }
  await page.goto('/media/e2e-series')
  await page.getByRole('combobox', { name: '选择季' }).selectOption('1')
  await page.getByText('批量记录', { exact: true }).click()
  await page.getByRole('button', { name: '标记整季' }).click()
  await expect(page.getByRole('button', { name: '本季已看完' })).toBeVisible()
  await page.getByRole('button', { name: '再刷' }).click()
  await page.getByRole('button', { name: `查看第 ${seasonRound.roundNumber} 刷` }).click()
  const seasonDialog = page.getByRole('dialog', { name: `第 ${seasonRound.roundNumber} 刷记录` })
  await expect(seasonDialog).toContainText('S01E01')
  await expectDialogWithinViewport(seasonDialog)
  await expectNoBlockingA11yViolations(page)
  await page.keyboard.press('Escape')
})

test('keeps rich details within desktop, tablet, and mobile viewports', async ({ page }) => {
  await login(page)
  for (const viewport of detailViewports) {
    await page.setViewportSize(viewport)
    await page.goto('/media/e2e-series')
    await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
    await expect(page.getByText('林见川')).toBeVisible()
    await page.getByRole('combobox', { name: '选择季' }).selectOption('2')
    await expect(page.getByText('重返北堤')).toBeVisible()
    await expectNoHorizontalOverflow(page)
    await expectNoFixedElementOverlap(page)
    if (viewport.width === 375) {
      const actionPosition = await page.locator('.personal-record-panel .form-actions').evaluate((element) => getComputedStyle(element).position)
      expect(actionPosition).toBe('static')
    }
  }
})

test('supports keyboard navigation, 200 percent zoom, and reduced motion', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await login(page)

  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: '跳到主要内容' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.locator('#main-content')).toBeFocused()

  await page.reload()
  const primaryNavigation = page.getByRole('navigation', { name: '主导航' })
  await expect(primaryNavigation).toBeVisible()
  await page.keyboard.press('Tab')
  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: 'video-record 首页' })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(primaryNavigation.getByRole('link', { name: '首页', exact: true })).toBeFocused()
  for (const name of ['影库', '日历', '统计', '设置']) {
    await page.keyboard.press('Tab')
    await expect(primaryNavigation.getByRole('link', { name, exact: true })).toBeFocused()
  }
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { level: 1, name: '设置' })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  for (let index = 0; index < 8; index += 1) await page.keyboard.press('Tab')
  const dialogSearch = page.getByRole('dialog', { name: '搜索影视' }).getByRole('searchbox', { name: '搜索影视' })
  await expect(dialogSearch).toBeFocused()
  await dialogSearch.fill('静默轨道')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: '搜索影视' })).toHaveCount(0)
  expect(await page.evaluate(() => ({
    ariaLabel: document.activeElement?.getAttribute('aria-label'),
    tagName: document.activeElement?.tagName,
  }))).toEqual({ ariaLabel: '搜索影视', tagName: 'INPUT' })
  await page.keyboard.press('Tab')
  await expect(page.getByRole('banner', { name: '应用导航' }).getByRole('button', { name: '记录', exact: true })).toBeFocused()

  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  await expectNoHorizontalOverflow(page)
  await expectNoFixedElementOverlap(page)
  const duration = await page.locator('.nav-link').first().evaluate((element) => getComputedStyle(element).transitionDuration)
  expect(Number.parseFloat(duration)).toBeLessThanOrEqual(0.001)
})

const detailViewports = [
  { width: 1440, height: 900 },
  { width: 768, height: 1024 },
  { width: 375, height: 812 },
]

async function expectDialogWithinViewport(dialog: Locator) {
  const bounds = await dialog.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return {
      top: rect.top,
      right: rect.right,
      bottom: rect.bottom,
      left: rect.left,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
    }
  })
  expect(bounds.left).toBeGreaterThanOrEqual(0)
  expect(bounds.top).toBeGreaterThanOrEqual(0)
  expect(bounds.right).toBeLessThanOrEqual(bounds.viewportWidth)
  expect(bounds.bottom).toBeLessThanOrEqual(bounds.viewportHeight)
}

async function expectControlMinHeight(control: Locator) {
  await expect(control).toBeVisible()
  const minHeight = await control.evaluate((element) => Number.parseFloat(getComputedStyle(element).minHeight))
  expect(minHeight).toBeGreaterThanOrEqual(44)
}

async function expectTokenStyle(
  control: Locator,
  property: 'backgroundColor' | 'borderTopColor' | 'color',
  token: `--${string}`,
) {
  const expected = await control.evaluate((_element, tokenName) => {
    const probe = document.createElement('span')
    probe.style.color = `var(${tokenName})`
    document.body.append(probe)
    const value = getComputedStyle(probe).color
    probe.remove()
    return value
  }, token)
  await expect.poll(() => control.evaluate(
    (element, propertyName) => getComputedStyle(element)[propertyName],
    property,
  )).toBe(expected)
}

async function computedBackgroundContrast(element: Locator) {
  return element.evaluate((target) => {
    const context = document.createElement('canvas').getContext('2d', { willReadFrequently: true })
    if (!context) throw new Error('Canvas 2D context is unavailable')

    const luminance = (color: string) => {
      context.clearRect(0, 0, 1, 1)
      context.fillStyle = color
      context.fillRect(0, 0, 1, 1)
      const channels = [...context.getImageData(0, 0, 1, 1).data].slice(0, 3).map((channel) => {
        const value = channel / 255
        return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
      })
      return (channels[0] ?? 0) * 0.2126 + (channels[1] ?? 0) * 0.7152 + (channels[2] ?? 0) * 0.0722
    }

    const loading = luminance(getComputedStyle(target).backgroundColor)
    const canvas = luminance(getComputedStyle(document.body).backgroundColor)
    return (Math.max(loading, canvas) + 0.05) / (Math.min(loading, canvas) + 0.05)
  })
}

async function holdRequest(page: Page, pattern: string, method: string) {
  let intercepted = false
  let count = 0
  let releaseRequest = () => {}
  let markRequestFinished = () => {}
  const heldRequest = new Promise<void>((resolve) => { releaseRequest = resolve })
  const requestFinished = new Promise<void>((resolve) => { markRequestFinished = resolve })
  const handler = async (route: Route) => {
    if (route.request().method() !== method) {
      await route.continue()
      return
    }
    intercepted = true
    count += 1
    await heldRequest
    try {
      await route.abort()
    } finally {
      markRequestFinished()
    }
  }
  await page.route(pattern, handler)

  return {
    requestCount: () => count,
    release: async () => {
      releaseRequest()
      if (intercepted) await requestFinished
      await page.unroute(pattern, handler)
    },
  }
}
