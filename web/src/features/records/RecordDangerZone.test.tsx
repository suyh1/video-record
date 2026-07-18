import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CurrentRound } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { RecordDangerZone } from './RecordDangerZone'

const round: CurrentRound = {
  roundId: 'round-1',
  mediaId: 'media-1',
  seasonNumber: null,
  roundNumber: 1,
  status: 'completed',
  rating: 8,
  note: '笔记',
  viewingMethod: '影院',
  watchedAt: '2026-07-13T12:00:00Z',
  startedAt: null,
  participantIds: [],
  version: 3,
  profileVersion: 2,
}

describe('RecordDangerZone', () => {
  beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

  it('clears optional fields after confirmation and ignores cancel', async () => {
    let clearCalls = 0
    server.use(http.post('*/api/v1/records/media-1/rounds/current/clear-fields', () => {
      clearCalls += 1
      return HttpResponse.json({ ...round, rating: null, note: null, viewingMethod: null, version: 4 })
    }))
    const onRoundChange = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter>
        <RecordDangerZone mediaID="media-1" round={round} onRoundChange={onRoundChange} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: '清空可选字段' }))
    await user.click(screen.getByRole('button', { name: '取消' }))
    expect(clearCalls).toBe(0)

    await user.click(screen.getByRole('button', { name: '清空可选字段' }))
    await user.click(screen.getByRole('button', { name: '确认' }))
    await waitFor(() => expect(onRoundChange).toHaveBeenCalledOnce())
    expect(clearCalls).toBe(1)
    expect(screen.getByRole('status')).toHaveTextContent('已清空评分、笔记和观看方式')
  })

  it('removes from library after confirmation', async () => {
    let removeCalls = 0
    server.use(http.post('*/api/v1/records/media-1/remove-from-library', () => {
      removeCalls += 1
      return new HttpResponse(null, { status: 204 })
    }))
    const user = userEvent.setup()
    renderWithQueryClient(
      <MemoryRouter>
        <RecordDangerZone mediaID="media-1" round={round} onRoundChange={() => undefined} />
      </MemoryRouter>,
    )

    await user.click(screen.getByRole('button', { name: '移出影库' }))
    await user.click(screen.getByRole('button', { name: '确认' }))
    await waitFor(() => expect(removeCalls).toBe(1))
  })
})
