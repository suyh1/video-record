import { expect, test } from '@playwright/test'

import { login } from './support'

test('reviews a synthetic sync candidate without exposing provider secrets', async ({ page }) => {
  let ignored = false
  await page.route('**/api/v1/sync/status', async (route) => {
    await route.fulfill({ json: { accounts: [{ id: 'account-1', provider: 'jellyfin', name: '家庭媒体库', enabled: true, pendingCandidates: ignored ? 0 : 1 }], pendingTotal: ignored ? 0 : 1 } })
  })
  await page.route('**/api/v1/sync/candidates**', async (route) => {
    if (route.request().method() === 'POST') {
      expect(route.request().headers()['x-csrf-token']).toBeTruthy()
      expect(route.request().headers()['idempotency-key']).toBeTruthy()
      ignored = true
      await route.fulfill({ json: { ...candidate, status: 'ignored' } })
      return
    }
    await route.fulfill({ json: ignored ? [] : [candidate] })
  })
  await login(page)
  await page.goto('/settings/sync')

  await expect(page.getByRole('heading', { name: '同步候选' })).toBeVisible()
  await expect(page.getByText('外部私密路径')).toHaveCount(0)
  await page.getByRole('button', { name: '忽略' }).click()
  await expect(page.getByText('没有需要核对的同步记录')).toBeVisible()
})

const candidate = {
  id: 'candidate-1',
  accountId: 'account-1',
  externalEventId: 'synthetic-event-1',
  status: 'possible',
  event: {
    playedAt: '2026-07-13T12:00:00Z',
    durationSeconds: 6000,
    positionSeconds: 5900,
    providerItemId: 'synthetic-item-1',
    mediaType: 'movie',
    title: '静默轨道',
    originalTitle: 'Silent Track',
    year: 2024,
  },
  evidence: [{ code: 'title_year', text: '标题和年份相同，需要人工确认' }],
  options: [{ mediaId: 'e2e-movie', mediaType: 'movie', title: '静默轨道', originalTitle: 'Silent Track', year: '2024' }],
  createdAt: '2026-07-13T12:00:00Z',
  updatedAt: '2026-07-13T12:00:00Z',
}
