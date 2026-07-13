import { expect, test } from '@playwright/test'

import { login } from './support'

test('creates a household member and records a shared viewing', async ({ page }) => {
  await login(page)
  await page.goto('/settings')

  await page.getByRole('button', { name: '添加成员' }).click()
  await page.getByLabel('用户名').fill('e2e-family')
  await page.getByLabel('初始密码').fill('Synthetic-family-pass-2026')
  await page.getByRole('button', { name: '创建成员' }).click()
  await expect(page.getByText('e2e-family')).toBeVisible()

  await page.goto('/media/e2e-movie')
  await page.getByRole('radio', { name: '看过' }).click()
  const expand = page.getByRole('button', { name: '更多记录选项' })
  if (await expand.isVisible()) await expand.click()
  await page.getByRole('checkbox', { name: 'e2e-family' }).check()
  await page.getByRole('button', { name: '保存记录' }).click()
  await expect(page.getByRole('status')).toContainText('记录已保存')
})
