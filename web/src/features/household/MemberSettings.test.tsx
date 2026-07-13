import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { MemberSettings } from './MemberSettings'

it('shows member roles and confirms before deactivation', async () => {
  let deactivated = false
  server.use(
    http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
    http.get('*/api/v1/household/members', () => HttpResponse.json([
      { id: 'admin-1', username: 'owner', role: 'admin', active: true },
      { id: 'member-1', username: 'family', role: 'member', active: true },
    ])),
    http.post('*/api/v1/household/members/member-1/deactivate', ({ request }) => {
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      deactivated = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  const user = userEvent.setup()
  renderWithQueryClient(<MemberSettings />)

  expect(await screen.findByRole('heading', { name: '家庭成员' })).toBeVisible()
  expect(screen.getByText('管理员')).toBeVisible()
  expect(screen.getAllByText('已启用')).toHaveLength(2)
  await user.click(screen.getByRole('button', { name: '停用 family' }))

  const dialog = screen.getByRole('dialog', { name: '停用家庭成员' })
  expect(dialog).toHaveTextContent('family')
  expect(screen.getByRole('button', { name: '确认停用' })).toHaveFocus()
  await user.click(screen.getByRole('button', { name: '确认停用' }))
  await waitFor(() => expect(deactivated).toBe(true))
})
