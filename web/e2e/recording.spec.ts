import { expect, test } from '@playwright/test'

import { baseURL, login } from './support'

test('keeps round recording usable when TMDB is unavailable', async ({ page }) => {
  await page.route('**/api/v1/tmdb/movie/2002**', (route) => route.fulfill({
    status: 502,
    json: { type: 'about:blank', status: 502, code: 'tmdb_unavailable', requestId: 'e2e-tmdb-failure' },
  }))
  await login(page)
  await page.goto('/media/e2e-movie')

  await expect(page.getByRole('heading', { level: 1, name: '静默轨道' })).toBeVisible()
  await expect(page.getByText('演员资料暂时不可用')).toBeVisible()
  await page.getByRole('button', { name: '更多记录选项' }).click()
  await page.getByLabel('私人笔记').fill('TMDB 暂不可用时仍可保存。')
  await page.getByRole('button', { name: '保存记录' }).click()
  await expect(page.getByRole('status')).toContainText('记录已保存')
})

test('completes a movie at an exact second and archives its private round on rewatch', async ({ page }) => {
  await login(page)
  await page.goto('/media/e2e-movie')

  await page.getByRole('radio', { name: '看过' }).click()
  await page.getByLabel('完成观看时间').fill('2026-07-13T20:30:45')
  if (!await page.getByRole('spinbutton', { name: '评分' }).isVisible()) {
    await page.getByRole('button', { name: '更多记录选项' }).click()
  }
  await page.getByRole('spinbutton', { name: '评分' }).fill('8.6')
  await page.getByLabel('观看方式').fill('家庭投影')
  await page.getByLabel('私人笔记').fill('合成记录，不包含真实用户数据。')
  await page.getByRole('button', { name: '保存记录' }).click()
  await expect(page.getByRole('status')).toContainText('记录已保存')

  const currentResponse = await page.request.get(`${baseURL}/api/v1/records/e2e-movie/rounds/current`)
  expect(currentResponse.ok()).toBeTruthy()
  const current = await currentResponse.json() as { roundNumber: number; watchedAt: string }
  expect(current.watchedAt).toBe('2026-07-13T12:30:45Z')

  await page.getByRole('button', { name: '再刷' }).click()
  await expect(page.getByText(`第 ${current.roundNumber} 刷`)).toBeVisible()
  await expect(page.getByRole('radio', { name: '在看' })).toBeChecked()
  await expect(page.getByLabel('私人笔记')).toHaveCount(0)

  await page.getByRole('button', { name: `查看第 ${current.roundNumber} 刷` }).click()
  const dialog = page.getByRole('dialog', { name: `第 ${current.roundNumber} 刷记录` })
  await expect(dialog).toContainText('2026-07-13 20:30:45')
  await expect(dialog).toContainText('8.6 / 10')
  await expect(dialog).toContainText('合成记录，不包含真实用户数据。')
})
