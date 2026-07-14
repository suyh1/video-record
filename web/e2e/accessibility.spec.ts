import { expect, test } from '@playwright/test'

import {
  expectNoBlockingA11yViolations,
  expectNoFixedElementOverlap,
  expectNoHorizontalOverflow,
  login,
} from './support'

test('has no blocking WCAG 2.2 AA violations on major pages', async ({ page }) => {
  await login(page)
  for (const path of ['/', '/library', '/calendar', '/stats', '/settings', '/settings/sync', '/media/e2e-series']) {
    await page.goto(path)
    await expect(page.locator('main')).toBeVisible()
    if (path === '/media/e2e-series') await expect(page.getByText('低潮线')).toBeVisible()
    await expectNoBlockingA11yViolations(page)
  }
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
  await expect(primaryNavigation.getByRole('link', { name: '首页', exact: true })).toBeFocused()
  for (const name of ['影库', '日历', '统计', '设置']) {
    await page.keyboard.press('Tab')
    await expect(primaryNavigation.getByRole('link', { name, exact: true })).toBeFocused()
  }
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { level: 1, name: '设置' })).toBeVisible()

  await page.reload()
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  for (let index = 0; index < 7; index += 1) await page.keyboard.press('Tab')
  const dialogSearch = page.getByRole('dialog', { name: '搜索影视' }).getByRole('searchbox', { name: '搜索影视' })
  await expect(dialogSearch).toBeFocused()
  await dialogSearch.fill('静默轨道')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: '搜索影视' })).toHaveCount(0)

  await page.evaluate(() => { document.documentElement.style.zoom = '2' })
  await expectNoHorizontalOverflow(page)
  const duration = await page.locator('.nav-link').first().evaluate((element) => getComputedStyle(element).transitionDuration)
  expect(Number.parseFloat(duration)).toBeLessThanOrEqual(0.001)
})

const detailViewports = [
  { width: 1440, height: 900 },
  { width: 768, height: 1024 },
  { width: 375, height: 812 },
]
