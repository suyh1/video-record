import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { SyncStatus } from './SyncStatus'

it('shows a compact, non-color sync summary with a review link', async () => {
  server.use(http.get('*/api/v1/sync/status', async () => {
    await delay(50)
    return HttpResponse.json({
      accounts: [
        {
          id: 'account-1',
          provider: 'jellyfin',
          name: 'Living Room',
          enabled: true,
          pendingCandidates: 2,
          lastRunStatus: 'succeeded',
          lastRunAt: '2026-07-13T08:30:00Z',
        },
        {
          id: 'account-2',
          provider: 'plex',
          name: 'Archive',
          enabled: false,
          pendingCandidates: 0,
        },
      ],
      pendingTotal: 2,
    })
  }))
  renderWithQueryClient(<MemoryRouter><SyncStatus /></MemoryRouter>)

  expect(screen.getByLabelText('正在加载媒体服务器同步状态')).toBeVisible()
  expect(await screen.findByRole('heading', { name: '媒体服务器同步' })).toBeVisible()
  expect(screen.getByText('Jellyfin · Living Room')).toBeVisible()
  expect(screen.getByText('同步成功')).toBeVisible()
  expect(screen.getByText('已停用')).toBeVisible()
  expect(screen.getByRole('link', { name: '核对 2 条候选' })).toHaveAttribute('href', '/settings/sync')
})

it('shows an actionable empty state and retries status errors', async () => {
  let requests = 0
  server.use(http.get('*/api/v1/sync/status', () => {
    requests++
    if (requests === 1) return HttpResponse.json({ code: 'internal_error' }, { status: 500 })
    return HttpResponse.json({ accounts: [], pendingTotal: 0 })
  }))
  const user = userEvent.setup()
  renderWithQueryClient(<MemoryRouter><SyncStatus /></MemoryRouter>)

  expect(await screen.findByRole('alert')).toHaveTextContent('无法读取同步状态')
  await user.click(screen.getByRole('button', { name: '重试' }))

  expect(await screen.findByText('还没有媒体服务器集成')).toBeVisible()
  await waitFor(() => expect(requests).toBe(2))
})
