import { expect, test } from '@playwright/test'

import { login } from './support'

test('advances one episode and supports undo', async ({ page }) => {
  await login(page)
  await page.goto('/media/e2e-series')

  await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
  await page.getByRole('button', { name: /推进下一集 S01E01/ }).click()
  await expect(page.getByRole('status')).toContainText('已推进至 S01E01')
  await expect(page.getByRole('button', { name: '将 S01E01 标为未看' })).toHaveAttribute('aria-pressed', 'true')

  await page.getByRole('button', { name: '撤销 S01E01' }).click()
  await expect(page.getByRole('button', { name: '标记 S01E01 已看' })).toHaveAttribute('aria-pressed', 'false')
})
