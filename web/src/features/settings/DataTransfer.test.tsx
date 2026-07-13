import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { DataTransfer } from './DataTransfer'

it('offers safe exports and reports imported records', async () => {
  let uploaded = false
  server.use(http.post('*/api/v1/data/import', ({ request }) => {
    expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
    expect(request.headers.get('Idempotency-Key')).toBeTruthy()
    expect(request.headers.get('Content-Type')).toContain('multipart/form-data; boundary=')
    uploaded = true
    return HttpResponse.json({
      importedRecords: 1,
      importedCollections: 0,
      failures: [{ recordId: 'media-2', code: 'external_identity_conflict' }],
    })
  }))
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  const user = userEvent.setup()
  renderWithQueryClient(<DataTransfer />)

  expect(screen.getByRole('link', { name: '导出 JSON' })).toHaveAttribute('href', '/api/v1/data/export?format=json')
  expect(screen.getByRole('link', { name: '导出 CSV' })).toHaveAttribute('href', '/api/v1/data/export?format=csv')
  const file = new File(['{"version":1,"records":[],"collections":[]}'], 'records.json', { type: 'application/json' })
  const fileInput = screen.getByLabelText('选择导入文件')
  await user.upload(fileInput, file)
  expect(fileInput).toHaveProperty('files.0.name', 'records.json')
  await user.click(screen.getByRole('button', { name: '导入数据' }))

  await waitFor(() => expect(uploaded).toBe(true))
  expect(screen.getByRole('status')).toHaveTextContent('已导入 1 条记录')
  expect(screen.getByText('media-2：外部身份冲突')).toBeVisible()
})
