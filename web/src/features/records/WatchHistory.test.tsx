import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { server } from '../../test/server'
import { WatchHistory } from './WatchHistory'

describe('WatchHistory', () => {
  it('invalidates episode progress after deleting a viewing event', async () => {
    sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
    server.use(
      http.delete('*/api/v1/records/series-1/events/event-1', () => new HttpResponse(null, { status: 204 })),
    )
    const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    queryClient.setQueryData(['episode-progress', 'series-1'], { version: 2 })
    render(
      <QueryClientProvider client={queryClient}>
        <WatchHistory
          mediaID="series-1"
          events={[{
            id: 'event-1', mediaId: 'series-1', watchedAt: '2026-07-13T12:00:00Z',
            viewingMethod: '', source: 'manual', completion: 100,
          }]}
        />
      </QueryClientProvider>,
    )

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '删除 2026年7月13日的观看事件' }))
    await user.click(screen.getByRole('button', { name: '确认删除观看事件' }))

    await waitFor(() => expect(
      queryClient.getQueryState(['episode-progress', 'series-1'])?.isInvalidated,
    ).toBe(true))
  })
})
