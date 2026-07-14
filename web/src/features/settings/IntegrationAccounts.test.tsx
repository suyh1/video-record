import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { IntegrationAccounts } from './IntegrationAccounts'

describe('IntegrationAccounts', () => {
  it('connects a provider without rendering credentials and disconnects after confirmation', async () => {
    const accounts = [{
      id: 'account-1', provider: 'jellyfin', name: '客厅 Jellyfin',
      credentialFingerprint: ['0123', '4567', '89ab', 'cdef'].join(''), enabled: true, locked: false,
      createdAt: '2026-07-13T00:00:00Z', updatedAt: '2026-07-13T00:00:00Z',
    }]
    let createPayload: unknown
    let disconnected = false
    server.use(
      http.get('*/api/v1/integrations/accounts', () => HttpResponse.json(accounts)),
      http.post('*/api/v1/integrations/accounts', async ({ request }) => {
        createPayload = await request.json()
        const created = {
          ...accounts[0]!, id: 'account-2', provider: 'emby', name: '书房 Emby',
          credentialFingerprint: ['fedc', 'ba98', '7654', '3210'].join(''),
        }
        accounts.push(created)
        return HttpResponse.json(created, { status: 201 })
      }),
      http.delete('*/api/v1/integrations/accounts/:accountID', () => {
        disconnected = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const user = userEvent.setup()
    renderWithQueryClient(<IntegrationAccounts />)

    expect(await screen.findByText('客厅 Jellyfin')).toBeVisible()
    expect(screen.queryByText('synthetic-provider-token')).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('服务类型'), 'emby')
    await user.type(screen.getByLabelText('账户名称'), '书房 Emby')
    await user.type(screen.getByLabelText('服务器地址'), 'https://emby.example.test')
    const token = screen.getByLabelText('访问令牌')
    expect(token).toHaveAttribute('type', 'password')
    await user.type(token, 'synthetic-provider-token')
    await user.type(screen.getByLabelText('用户 ID'), 'emby-user')
    await user.type(screen.getByLabelText('服务器时区'), 'Asia/Shanghai')
    await user.click(screen.getByRole('button', { name: '连接媒体服务器' }))

    await waitFor(() => expect(createPayload).toEqual({
      provider: 'emby', name: '书房 Emby', baseUrl: 'https://emby.example.test',
      token: 'synthetic-provider-token', userId: 'emby-user', timezone: 'Asia/Shanghai',
    }))
    expect(await screen.findByText('书房 Emby')).toBeVisible()
    expect(screen.queryByText('synthetic-provider-token')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '断开 客厅 Jellyfin' }))
    const confirm = await screen.findByRole('button', { name: '确认断开' })
    expect(confirm).toHaveFocus()
    await user.click(confirm)
    await waitFor(() => expect(disconnected).toBe(true))
  })
})
