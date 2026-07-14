import { expect, test } from '@playwright/test'

import { login, mockTMDB } from './support'

test('searches, records, and preserves a repeat viewing', async ({ page }) => {
  await mockTMDB(page)
  await login(page)

  await page.getByRole('searchbox', { name: '搜索影视' }).click()
  await page.getByRole('dialog', { name: '搜索影视' }).getByRole('searchbox').fill('静默轨道')
  await page.getByRole('button', { name: /静默轨道/ }).click()
  await expect(page.getByRole('heading', { level: 1, name: '静默轨道' })).toBeVisible()

  await page.getByRole('radio', { name: '看过' }).click()
  await page.getByRole('button', { name: '更多记录选项' }).click()
  await page.getByRole('spinbutton', { name: '评分' }).fill('8.6')
  await page.getByLabel('私人笔记').fill('合成记录，不包含真实用户数据。')
  await page.getByRole('button', { name: '保存记录' }).click()
  await expect(page.getByRole('status')).toContainText('记录已保存')
  await expect(page.getByText('1 次记录')).toBeVisible()

  await page.getByRole('button', { name: '再看一次' }).click()
  await expect(page.getByText('2 次记录')).toBeVisible()
})
