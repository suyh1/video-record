import { expect, test } from '@playwright/test'

import {
  baseURL,
  controlSyntheticTMDB,
  expectImageLoaded,
  login,
  syntheticTMDBCounts,
} from './support'

test('loads live cast and seasons, persists one sparse episode, and supports undo', async ({ page }) => {
  await login(page)
  const before = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress`)
  await expect(before.json()).resolves.toMatchObject({ episodes: [] })
  await page.goto('/media/e2e-series')

  await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
  await expectImageLoaded(page.getByRole('img', { name: '潮汐档案 背景' }))
  await expectImageLoaded(page.getByRole('img', { name: '潮汐档案 海报' }))
  await expect(page.getByRole('region', { name: '主要演员' })).toContainText('林见川')
  await expect(page.getByText('低潮线')).toBeVisible()
  await expect(page.getByRole('combobox', { name: '选择季' })).toHaveValue('1')
  await expect(page.getByRole('textbox', { name: '私人标签' })).toHaveCount(0)
  await page.getByRole('button', { name: /推进下一集 S01E01/ }).click()
  await expect(page.getByRole('status')).toContainText('已推进至 S01E01')
  await expect(page.getByRole('button', { name: '将 S01E01 标为未看' })).toHaveAttribute('aria-pressed', 'true')
  const saved = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress`)
  const savedProgress = await saved.json() as { episodes: Array<{ sourceId: string; watched: boolean }> }
  expect(savedProgress.episodes).toHaveLength(1)
  expect(savedProgress.episodes[0]).toMatchObject({ sourceId: '1101', watched: true })

  await page.getByRole('button', { name: '撤销 S01E01' }).click()
  await expect(page.getByRole('button', { name: '标记 S01E01 已看' })).toHaveAttribute('aria-pressed', 'false')
})

test('switches seasons and records a whole live season', async ({ page }) => {
  await login(page)
  await page.goto('/media/e2e-series')

  await page.getByRole('combobox', { name: '选择季' }).selectOption('2')
  await expect(page.getByText('重返北堤')).toBeVisible()
  await expect(page.getByText('潮汐尽头')).toBeVisible()
  await page.getByText('批量记录', { exact: true }).click()
  await page.getByRole('button', { name: '标记整季' }).click()
  await expect(page.getByRole('button', { name: '将 S02E01 标为未看' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('button', { name: '将 S02E02 标为未看' })).toHaveAttribute('aria-pressed', 'true')

  const saved = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress`)
  const progress = await saved.json() as { episodes: Array<{ sourceId?: string; watched: boolean }> }
  expect(progress.episodes.filter((episode) => episode.watched).map((episode) => episode.sourceId)).toEqual(['1201', '1202'])
})

test('reuses the six-hour server cache for repeated authenticated reads', async ({ page }) => {
  await login(page)
  await controlSyntheticTMDB(page, [], true)

  const first = await page.request.get(`${baseURL}/api/v1/tmdb/movie/3003`)
  const second = await page.request.get(`${baseURL}/api/v1/tmdb/movie/3003`)
  expect(first.ok()).toBeTruthy()
  expect(second.ok()).toBeTruthy()
  const counts = await syntheticTMDBCounts(page)
  expect(counts['/3/movie/3003']).toBe(1)
})
