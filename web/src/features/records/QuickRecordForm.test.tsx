import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { RecordState } from '../../api/types'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { QuickRecordForm } from './QuickRecordForm'

const initialRecord = {
  mediaId: 'media-1',
  status: 'none' as const,
  rating: null,
  note: null,
  watchedAt: null,
  viewingMethod: null,
  version: 0,
}

describe('QuickRecordForm', () => {
  beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

  it('records wishlist in two clear actions', async () => {
    const requestBodies: unknown[] = []
    let attempts = 0
    server.use(
      http.put('*/api/v1/records/media-1', async ({ request }) => {
        attempts += 1
        expect(request.headers.get('If-Match')).toBe(attempts === 1 ? '"0"' : '"1"')
        expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
        expect(request.headers.get('Idempotency-Key')).toBeTruthy()
        requestBodies.push(await request.json())
        return HttpResponse.json(
          attempts === 1 ? { ...initialRecord, status: 'wishlist', version: 1 } : { ...initialRecord, version: 2 },
          { headers: { ETag: attempts === 1 ? '"1"' : '"2"' } },
        )
      }),
    )
    const onSaved = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<QuickRecordForm record={initialRecord} now={new Date('2026-07-13T09:00:00Z')} onSaved={onSaved} />)

    await user.click(screen.getByRole('radio', { name: '想看' }))
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce())
    expect(requestBodies[0]).toEqual({ status: 'wishlist' })
    expect(screen.getByRole('status')).toHaveTextContent('记录已保存')

    await user.click(screen.getByRole('button', { name: '撤销刚才的修改' }))

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(2))
    expect(requestBodies[1]).toEqual({ status: 'none' })
  })

  it('defaults completed to today and progressively reveals optional fields', async () => {
    const user = userEvent.setup()
    renderWithQueryClient(<QuickRecordForm record={initialRecord} now={new Date('2026-07-13T09:00:00Z')} onSaved={() => undefined} />)

    expect(screen.queryByLabelText('评分')).not.toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: '看过' }))
    expect(screen.getByLabelText('观看日期')).toHaveValue('2026-07-13')
    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    expect(screen.getByLabelText('评分')).toBeVisible()
    expect(screen.getByLabelText('私人笔记')).toBeVisible()
    expect(screen.getByLabelText('观看方式')).toBeVisible()
  })

  it('preserves entered values when the network fails', async () => {
    server.use(http.put('*/api/v1/records/media-1', () => HttpResponse.error()))
    const user = userEvent.setup()
    renderWithQueryClient(<QuickRecordForm record={initialRecord} now={new Date('2026-07-13T09:00:00Z')} onSaved={() => undefined} />)

    await user.click(screen.getByRole('radio', { name: '在看' }))
    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.type(screen.getByLabelText('私人笔记'), '看到沙虫出现')
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
    expect(screen.getByLabelText('私人笔记')).toHaveValue('看到沙虫出现')
  })

  it('resets visible fields from the undo response after the parent rerenders', async () => {
    const wishlistRecord = { ...initialRecord, status: 'wishlist' as const, version: 1 }
    let attempts = 0
    server.use(http.put('*/api/v1/records/media-1', () => {
      attempts += 1
      return HttpResponse.json(attempts === 1
        ? { ...wishlistRecord, note: '临时笔记', version: 2 }
        : { ...wishlistRecord, note: null, version: 3 })
    }))
    const user = userEvent.setup()

    function ControlledForm() {
      const [record, setRecord] = useState<RecordState>(wishlistRecord)
      return <QuickRecordForm record={record} now={new Date('2026-07-13T09:00:00Z')} onSaved={setRecord} />
    }

    renderWithQueryClient(<ControlledForm />)
    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.type(screen.getByLabelText('私人笔记'), '临时笔记')
    await user.click(screen.getByRole('button', { name: '保存记录' }))
    await user.click(await screen.findByRole('button', { name: '撤销刚才的修改' }))

    await waitFor(() => expect(screen.getByLabelText('私人笔记')).toHaveValue(''))
  })

  it('keeps the draft and reapplies it against the latest ETag after a conflict', async () => {
    let attempts = 0
    server.use(
      http.put('*/api/v1/records/media-1', ({ request }) => {
        attempts += 1
        if (attempts === 1) {
          return HttpResponse.json(
            { type: 'about:blank', status: 409, code: 'version_conflict', requestId: 'request-1' },
            { status: 409, headers: { ETag: '"3"' } },
          )
        }
        expect(request.headers.get('If-Match')).toBe('"3"')
        return HttpResponse.json({ ...initialRecord, status: 'watching', note: '保留这段文字', version: 4 }, { headers: { ETag: '"4"' } })
      }),
    )
    const onSaved = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(<QuickRecordForm record={initialRecord} now={new Date('2026-07-13T09:00:00Z')} onSaved={onSaved} />)

    await user.click(screen.getByRole('radio', { name: '在看' }))
    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.type(screen.getByLabelText('私人笔记'), '保留这段文字')
    await user.click(screen.getByRole('button', { name: '保存记录' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('记录已在其他位置更新')
    expect(screen.getByLabelText('私人笔记')).toHaveValue('保留这段文字')

    await user.click(screen.getByRole('button', { name: '使用最新版本重试' }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce())
    expect(attempts).toBe(2)
  })

  it('includes selected household participants in a completed event', async () => {
    let requestBody: unknown
    server.use(http.put('*/api/v1/records/media-1', async ({ request }) => {
      requestBody = await request.json()
      return HttpResponse.json({ ...initialRecord, status: 'completed', version: 1 })
    }))
    const user = userEvent.setup()
    renderWithQueryClient(
      <QuickRecordForm
        record={initialRecord}
        now={new Date('2026-07-13T09:00:00Z')}
        participants={[{ id: 'member-1', username: 'family', role: 'member', active: true }]}
        onSaved={() => undefined}
      />,
    )

    await user.click(screen.getByRole('radio', { name: '看过' }))
    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.click(screen.getByRole('checkbox', { name: 'family' }))
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    await waitFor(() => expect(requestBody).toMatchObject({ participantIds: ['member-1'] }))
  })

  it('creates an explicit immutable rewatch event for a completed record', async () => {
    const completedRecord = {
      ...initialRecord,
      status: 'completed' as const,
      watchedAt: '2026-07-12T12:00:00Z',
      viewingMethod: '家庭电视',
      version: 3,
    }
    let requestBody: unknown
    server.use(http.post('*/api/v1/records/media-1/events', async ({ request }) => {
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      expect(request.headers.get('Idempotency-Key')).toBeTruthy()
      requestBody = await request.json()
      return HttpResponse.json({
        id: 'rewatch-1', mediaId: 'media-1', watchedAt: '2026-07-13T12:00:00Z',
        viewingMethod: '家庭电视', source: 'manual', completion: 100,
      }, { status: 201 })
    }))
    const onRewatched = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(
      <QuickRecordForm
        record={completedRecord}
        now={new Date('2026-07-13T09:00:00Z')}
        onSaved={() => undefined}
        onRewatched={onRewatched}
      />,
    )

    await user.clear(screen.getByLabelText('观看日期'))
    await user.type(screen.getByLabelText('观看日期'), '2026-07-13')
    await user.click(screen.getByRole('button', { name: '再看一次' }))

    await waitFor(() => expect(onRewatched).toHaveBeenCalledOnce())
    expect(requestBody).toEqual({ watchedAt: '2026-07-13T12:00:00.000Z', viewingMethod: '家庭电视' })
    expect(screen.getByRole('status')).toHaveTextContent('重复观看已记录')
  })
})
