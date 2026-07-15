import { expect, test } from '@playwright/test'

import {
  baseURL,
  controlSyntheticTMDB,
  expectImageLoaded,
  login,
  syntheticTMDBCounts,
} from './support'

test('loads a selected season, records current time from the circle, and supports undo', async ({ page }) => {
  await login(page)
  const before = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress?seasonNumber=1`)
  const beforeProgress = await before.json() as {
    seasonNumber: number
    watchedEpisodes: number
    episodes: Array<{ watched: boolean }>
  }
  expect(beforeProgress).toMatchObject({ seasonNumber: 1, watchedEpisodes: 0 })
  expect(beforeProgress.episodes.every((episode) => !episode.watched)).toBe(true)
  await page.goto('/media/e2e-series')

  await expect(page.getByRole('heading', { level: 1, name: '潮汐档案' })).toBeVisible()
  await expectImageLoaded(page.getByRole('img', { name: '潮汐档案 背景' }))
  await expectImageLoaded(page.getByRole('img', { name: '潮汐档案 海报' }))
  await expect(page.getByRole('region', { name: '主要演员' })).toContainText('林见川')
  await expect(page.getByText('低潮线')).toBeVisible()
  await expect(page.getByRole('combobox', { name: '选择季' })).toHaveValue('1')
  await page.getByRole('button', { name: '标记 S01E01 已看' }).click()
  await expect(page.getByRole('button', { name: '将 S01E01 标为未看' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('button', { name: /修改 S01E01 观看时间/ })).toContainText(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)

  const saved = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress?seasonNumber=1`)
  const savedProgress = await saved.json() as { episodes: Array<{ sourceId: string; watched: boolean }> }
  expect(savedProgress.episodes.filter((episode) => episode.watched)).toMatchObject([
    { sourceId: '1101', watched: true },
  ])

  await page.getByRole('button', { name: '将 S01E01 标为未看' }).click()
  await expect(page.getByRole('button', { name: '标记 S01E01 已看' })).toHaveAttribute('aria-pressed', 'false')
})

test('sets an unwatched episode time directly and retains a rejected future value', async ({ page }) => {
  await login(page)
  await page.goto('/media/e2e-series')
  await page.getByRole('combobox', { name: '选择季' }).selectOption('2')
  await expect(page.getByText('重返北堤')).toBeVisible()

  const firstTimeButton = page.getByRole('button', { name: '设置 S02E01 观看时间' })
  await firstTimeButton.focus()
  await page.keyboard.press('Enter')
  const firstTime = page.getByRole('textbox', { name: 'S02E01 观看时间' })
  await expect(firstTime).toBeFocused()
  await firstTime.fill('2026-07-13T21:22:23')
  await firstTime.press('Enter')
  await expect(page.getByRole('button', { name: /修改 S02E01 观看时间/ })).toContainText('2026-07-13 21:22:23')

  const secondTimeButton = page.getByRole('button', { name: '设置 S02E02 观看时间' })
  await secondTimeButton.focus()
  await page.keyboard.press('Enter')
  const secondTime = page.getByRole('textbox', { name: 'S02E02 观看时间' })
  await expect(secondTime).toBeFocused()
  await secondTime.fill('2099-01-01T00:00:01')
  await secondTime.press('Enter')
  await expect(page.getByRole('alert')).toContainText('观看时间不能晚于当前时间')
  await expect(secondTime).toHaveValue('2099-01-01T00:00:01')
  await expect(page.getByRole('button', { name: '标记 S02E02 已看' })).toHaveAttribute('aria-pressed', 'false')
})

test('archives only season two and opens its episode detail entirely by keyboard', async ({ page }) => {
  await login(page)
  const seasonOneBefore = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress?seasonNumber=1`)
  const seasonOneSnapshot = await seasonOneBefore.json()
  await page.goto('/media/e2e-series')
  await page.getByRole('combobox', { name: '选择季' }).selectOption('2')

  const firstEpisodeRow = page.getByRole('listitem').filter({ hasText: 'S02E01' })
  await firstEpisodeRow.locator('.episode-time-button').click()
  await page.getByRole('textbox', { name: 'S02E01 观看时间' }).fill('2026-07-13T21:22:23')
  await page.getByRole('textbox', { name: 'S02E01 观看时间' }).press('Enter')

  await page.getByText('批量记录', { exact: true }).click()
  await page.getByRole('button', { name: '标记整季' }).click()
  await expect(page.getByRole('button', { name: '本季已看完' })).toBeVisible()

  const secondEpisodeRow = page.getByRole('listitem').filter({ hasText: 'S02E02' })
  await secondEpisodeRow.locator('.episode-time-button').click()
  await page.getByRole('textbox', { name: 'S02E02 观看时间' }).fill('2026-07-13T22:23:24')
  await page.getByRole('textbox', { name: 'S02E02 观看时间' }).press('Enter')
  await expect(secondEpisodeRow.locator('.episode-time-button')).toContainText('2026-07-13 22:23:24')

  if (!await page.getByRole('spinbutton', { name: '评分' }).isVisible()) {
    await page.getByRole('button', { name: '更多记录选项' }).click()
  }
  await page.getByRole('spinbutton', { name: '评分' }).fill('9.1')
  await page.getByLabel('观看方式').fill('客厅电视')
  await page.getByLabel('私人笔记').fill('只属于第二季的记录。')
  await page.getByRole('button', { name: '保存记录' }).click()
  await expect(page.getByRole('status')).toContainText('记录已保存')

  const currentResponse = await page.request.get(`${baseURL}/api/v1/records/e2e-series/rounds/current?seasonNumber=2`)
  const current = await currentResponse.json() as { roundNumber: number }
  await page.getByRole('button', { name: '再刷' }).click()
  const view = page.getByRole('button', { name: `查看第 ${current.roundNumber} 刷` })
  await expect(view).toBeVisible()
  await view.focus()
  await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog', { name: `第 ${current.roundNumber} 刷记录` })
  await expect(dialog).toContainText('只属于第二季的记录。')
  await expect(dialog).toContainText('S02E01')
  await expect(dialog).toContainText('重返北堤')
  await expect(dialog).toContainText('潮汐尽头')
  await expect(dialog).not.toContainText('未命名')
  await expect(dialog).toContainText('2026-07-13 21:22:23')
  await expect(dialog).toContainText('2026-07-13 22:23:24')
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)

  const seasonOneAfter = await page.request.get(`${baseURL}/api/v1/records/e2e-series/progress?seasonNumber=1`)
  await expect(seasonOneAfter.json()).resolves.toEqual(seasonOneSnapshot)
  await page.getByRole('combobox', { name: '选择季' }).selectOption('1')
  await expect(page.getByRole('button', { name: '标记 S01E01 已看' })).toBeVisible()
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
