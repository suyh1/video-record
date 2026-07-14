import { expect, test } from '@playwright/test'

import { expectImageLoaded, expectNoHorizontalOverflow, login } from './support'

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
