import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect, it, vi } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { RecordSharingEditor } from './RecordSharingEditor'

it('keeps ratings and reviews private until the user explicitly shares them', async () => {
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  let requestBody: unknown
  server.use(
    http.get('*/api/v1/household/records/media-1/sharing', () => HttpResponse.json({
      mediaId: 'media-1', shareRating: false, shareReview: false, sharedReview: null, version: 3,
    })),
    http.put('*/api/v1/household/records/media-1/sharing', async ({ request }) => {
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      expect(request.headers.get('Idempotency-Key')).toBeTruthy()
      requestBody = await request.json()
      return HttpResponse.json({
        mediaId: 'media-1', shareRating: true, shareReview: true,
        sharedReview: '值得和家人一起看', version: 4,
      }, { headers: { ETag: '"4"' } })
    }),
  )
  const onVersionChange = vi.fn()
  const user = userEvent.setup()
  renderWithQueryClient(<RecordSharingEditor mediaID="media-1" version={3} onVersionChange={onVersionChange} />)

  const rating = await screen.findByRole('checkbox', { name: '向家庭公开评分' })
  const review = screen.getByRole('checkbox', { name: '向家庭公开短评' })
  expect(rating).not.toBeChecked()
  expect(review).not.toBeChecked()
  expect(screen.queryByRole('textbox', { name: '家庭短评' })).not.toBeInTheDocument()

  await user.click(rating)
  await user.click(review)
  await user.type(screen.getByRole('textbox', { name: '家庭短评' }), '值得和家人一起看')
  await user.click(screen.getByRole('button', { name: '保存家庭公开设置' }))

  await waitFor(() => expect(requestBody).toEqual({
    shareRating: true,
    shareReview: true,
    sharedReview: '值得和家人一起看',
    expectedVersion: 3,
  }))
  expect(onVersionChange).toHaveBeenCalledWith(4)
  expect(screen.getByRole('status')).toHaveTextContent('家庭公开设置已保存')
})
