import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { expect, it } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { BackupRestore } from './BackupRestore'

it('creates, downloads, and confirms restoration of system backups', async () => {
  let created = false
  let restored = false
  server.use(
    http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
    http.get('*/api/v1/backups', () => HttpResponse.json([backup('video-record-existing.vrbackup')])),
    http.post('*/api/v1/backups', ({ request }) => {
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      expect(request.headers.get('Idempotency-Key')).toBeTruthy()
      created = true
      return HttpResponse.json(backup('video-record-new.vrbackup'), { status: 201 })
    }),
    http.post('*/api/v1/restore', async ({ request }) => {
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      expect(request.headers.get('Idempotency-Key')).toBeTruthy()
      expect(request.headers.get('Content-Type')).toContain('multipart/form-data; boundary=')
      restored = true
      return HttpResponse.json({
        preRestoreBackup: 'video-record-before-restore.vrbackup',
        warnings: ['integrations_locked'],
      })
    }),
  )
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  const user = userEvent.setup()
  renderWithQueryClient(<BackupRestore />)

  expect(await screen.findByRole('link', { name: '下载 video-record-existing.vrbackup' })).toHaveAttribute(
    'href',
    '/api/v1/backups/video-record-existing.vrbackup',
  )
  await user.click(screen.getByRole('button', { name: '创建系统备份' }))
  await waitFor(() => expect(created).toBe(true))
  expect(await screen.findByText('备份已创建')).toBeVisible()

  const file = new File(['backup'], 'restore.vrbackup', { type: 'application/vnd.video-record.backup' })
  await user.upload(screen.getByLabelText('选择系统备份'), file)
  await user.click(screen.getByRole('button', { name: '恢复此备份' }))
  const dialog = screen.getByRole('dialog', { name: '恢复系统备份' })
  expect(dialog).toHaveTextContent('自动创建当前数据库快照')
  expect(screen.getByRole('button', { name: '确认恢复' })).toHaveFocus()
  await user.click(screen.getByRole('button', { name: '确认恢复' }))

  await waitFor(() => expect(restored).toBe(true))
  expect(await screen.findByText('系统备份已恢复')).toBeVisible()
  expect(screen.getByRole('alert')).toHaveTextContent('集成凭据已锁定')
})

it('keeps the selected backup and announces restore failures', async () => {
  server.use(
    http.get('*/api/v1/auth/me', () => HttpResponse.json({ id: 'admin-1', username: 'owner', role: 'admin' })),
    http.get('*/api/v1/backups', () => HttpResponse.json([])),
    http.post('*/api/v1/restore', () => HttpResponse.json({ code: 'restore_failed' }, { status: 500 })),
  )
  sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token')
  const user = userEvent.setup()
  renderWithQueryClient(<BackupRestore />)

  const file = new File(['invalid backup'], 'restore.vrbackup', { type: 'application/vnd.video-record.backup' })
  const input = await screen.findByLabelText('选择系统备份')
  await user.upload(input, file)
  await user.click(screen.getByRole('button', { name: '恢复此备份' }))
  await user.click(screen.getByRole('button', { name: '确认恢复' }))

  expect(await screen.findByRole('alert')).toHaveTextContent('恢复失败')
  expect(input).toHaveProperty('files.0.name', 'restore.vrbackup')
})

function backup(filename: string) {
  return {
    filename,
    bytes: 2048,
    manifest: {
      formatVersion: 1,
      schemaVersion: 9,
      createdAt: '2026-07-13T08:30:00Z',
      databaseSha256: 'synthetic-checksum',
      databaseBytes: 1024,
      requiresEncryptionKey: false,
    },
  }
}
