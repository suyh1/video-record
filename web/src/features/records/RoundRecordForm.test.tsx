import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CurrentRound } from '../../api/types'
import { toDateTimeLocalValue } from '../../lib/dateTime'
import { renderWithQueryClient } from '../../test/render'
import { server } from '../../test/server'
import { RoundRecordForm } from './RoundRecordForm'

const now = new Date(2026, 6, 14, 17, 8, 9)

const movieRound: CurrentRound = {
  roundId: '',
  mediaId: 'media-1',
  seasonNumber: null,
  roundNumber: 1,
  status: 'none',
  rating: null,
  note: null,
  viewingMethod: null,
  watchedAt: null,
  version: 0,
  profileVersion: 7,
}

const seasonRound: CurrentRound = {
  ...movieRound,
  roundId: 'round-season-2',
  seasonNumber: 2,
  status: 'watching',
  version: 3,
}

describe('RoundRecordForm', () => {
  beforeEach(() => sessionStorage.setItem('video-record.csrf-token', 'csrf-test-token'))

  it('labels movie and season records in their actual scope without a rewatch action', () => {
    const { unmount } = renderWithQueryClient(
      <RoundRecordForm round={movieRound} now={now} onSaved={() => undefined} />,
    )
    expect(screen.getByRole('heading', { name: '个人记录' })).toBeVisible()
    expect(screen.queryByRole('button', { name: /再刷|再看一次/ })).not.toBeInTheDocument()

    unmount()
    renderWithQueryClient(
      <RoundRecordForm round={seasonRound} now={now} onSaved={() => undefined} />,
    )
    expect(screen.getByRole('heading', { name: '第 2 季个人记录' })).toBeVisible()
    expect(screen.queryByRole('button', { name: /再刷|再看一次/ })).not.toBeInTheDocument()
  })

  it('defaults a completed movie to a required second-precision local datetime', async () => {
    const user = userEvent.setup()
    renderWithQueryClient(
      <RoundRecordForm round={movieRound} now={now} onSaved={() => undefined} />,
    )

    await user.click(screen.getByRole('radio', { name: '看过' }))
    const watchedAt = screen.getByLabelText('完成观看时间')
    expect(watchedAt).toHaveAttribute('type', 'datetime-local')
    expect(watchedAt).toHaveAttribute('step', '1')
    expect(watchedAt).toHaveAttribute('max', toDateTimeLocalValue(now))
    expect(watchedAt).toHaveValue(`${toDateTimeLocalValue(now)}.000`)

    await user.clear(watchedAt)
    await user.click(screen.getByRole('button', { name: '保存记录' }))
    expect(await screen.findByText('请选择完成观看时间')).toBeVisible()
  })

  it('rejects a future movie completion time without sending or clearing the input', async () => {
    const request = vi.fn()
    server.use(http.put('*/api/v1/records/media-1/rounds/current', request))
    const user = userEvent.setup()
    renderWithQueryClient(
      <RoundRecordForm round={movieRound} now={now} onSaved={() => undefined} />,
    )

    await user.click(screen.getByRole('radio', { name: '看过' }))
    const watchedAt = screen.getByLabelText('完成观看时间')
    fireEvent.change(watchedAt, { target: { value: '2026-07-14T17:08:10' } })
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    expect(await screen.findByText('完成观看时间不能晚于当前时间')).toBeVisible()
    expect(watchedAt).toHaveValue('2026-07-14T17:08:10.000')
    expect(request).not.toHaveBeenCalled()
  })

  it('saves private fields and participants to the current movie round with its own version', async () => {
    let requestBody: unknown
    server.use(http.put('*/api/v1/records/media-1/rounds/current', async ({ request }) => {
      expect(request.headers.get('If-Match')).toBe('"0"')
      expect(request.headers.get('If-Match')).not.toBe('"7"')
      expect(request.headers.get('X-CSRF-Token')).toBe('csrf-test-token')
      expect(request.headers.get('Idempotency-Key')).toBeTruthy()
      requestBody = await request.json()
      return HttpResponse.json({
        ...movieRound,
        roundId: 'round-movie-1',
        status: 'completed',
        rating: 8.5,
        note: '银幕声音很好',
        viewingMethod: '影院',
        watchedAt: now.toISOString(),
        version: 1,
      })
    }))
    const onSaved = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(
      <RoundRecordForm
        round={movieRound}
        now={now}
        participants={[{ id: 'member-1', username: 'family', role: 'member', active: true }]}
        onSaved={onSaved}
      />,
    )

    await user.click(screen.getByRole('radio', { name: '看过' }))
    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.type(screen.getByLabelText('评分'), '8.5')
    await user.type(screen.getByLabelText('观看方式'), '影院')
    await user.click(screen.getByRole('checkbox', { name: 'family' }))
    await user.type(screen.getByLabelText('私人笔记'), '银幕声音很好')
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce())
    expect(requestBody).toEqual({
      status: 'completed',
      watchedAt: now.toISOString(),
      participantIds: ['member-1'],
      rating: 8.5,
      note: '银幕声音很好',
      viewingMethod: '影院',
    })
  })

  it('keeps the season draft and reapplies it with the latest ETag after a conflict', async () => {
    let attempts = 0
    server.use(http.put('*/api/v1/records/media-1/rounds/current', async ({ request }) => {
      attempts += 1
      expect(new URL(request.url).searchParams.get('seasonNumber')).toBe('2')
      if (attempts === 1) {
        expect(request.headers.get('If-Match')).toBe('"3"')
        return HttpResponse.json(
          { type: 'about:blank', status: 409, code: 'version_conflict', requestId: 'request-1' },
          { status: 409, headers: { ETag: '"6"' } },
        )
      }
      expect(request.headers.get('If-Match')).toBe('"6"')
      return HttpResponse.json({ ...seasonRound, note: '保留这一季的笔记', version: 7 })
    }))
    const onSaved = vi.fn()
    const user = userEvent.setup()
    renderWithQueryClient(
      <RoundRecordForm round={seasonRound} now={now} onSaved={onSaved} />,
    )

    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.type(screen.getByLabelText('私人笔记'), '保留这一季的笔记')
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('记录已在其他位置更新')
    expect(screen.getByLabelText('私人笔记')).toHaveValue('保留这一季的笔记')
    await user.click(screen.getByRole('button', { name: '使用最新版本重试' }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledOnce())
    expect(attempts).toBe(2)
  })

  it('preserves a draft after a network failure', async () => {
    server.use(http.put('*/api/v1/records/media-1/rounds/current', () => HttpResponse.error()))
    const user = userEvent.setup()
    renderWithQueryClient(
      <RoundRecordForm round={seasonRound} now={now} onSaved={() => undefined} />,
    )

    await user.click(screen.getByRole('button', { name: '更多记录选项' }))
    await user.type(screen.getByLabelText('私人笔记'), '网络断开也不要丢失')
    await user.click(screen.getByRole('button', { name: '保存记录' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('保存失败')
    expect(screen.getByLabelText('私人笔记')).toHaveValue('网络断开也不要丢失')
  })

  it('resets all visible fields when a successful rewatch supplies a new current round', async () => {
    const user = userEvent.setup()

    function ControlledForm() {
      const [round, setRound] = useState<CurrentRound>({
        ...movieRound,
        roundId: 'round-movie-1',
        roundNumber: 1,
        status: 'completed',
        rating: 9,
        note: '上一刷',
        viewingMethod: '影院',
        watchedAt: '2026-07-13T12:00:01Z',
        version: 4,
      })
      return (
        <>
          <button type="button" onClick={() => setRound({
            ...movieRound,
            roundId: 'round-movie-2',
            roundNumber: 2,
            status: 'watching',
            version: 1,
          })}>
            模拟再刷成功
          </button>
          <RoundRecordForm round={round} now={now} onSaved={setRound} />
        </>
      )
    }

    renderWithQueryClient(<ControlledForm />)
    expect(screen.getByLabelText('评分')).toHaveValue(9)
    await user.clear(screen.getByLabelText('私人笔记'))
    await user.type(screen.getByLabelText('私人笔记'), '尚未提交的改动')

    await user.click(screen.getByRole('button', { name: '模拟再刷成功' }))

    expect(screen.getByRole('radio', { name: '在看' })).toBeChecked()
    expect(screen.queryByLabelText('完成观看时间')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('评分')).not.toBeInTheDocument()
    expect(screen.queryByText('尚未提交的改动')).not.toBeInTheDocument()
  })
})
