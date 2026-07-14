import { fireEvent, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef, useState } from 'react'
import { expect, it, vi } from 'vitest'

import { renderWithQueryClient } from '../../test/render'
import { EpisodeTimeEditor } from './EpisodeTimeEditor'

const now = new Date(2026, 6, 14, 17, 8, 9)

it('keeps a future value visible and blocks confirmation', async () => {
  const onConfirm = vi.fn()
  const user = userEvent.setup()
  renderWithQueryClient(
    <EpisodeTimeEditor
      episodeLabel="S02E03"
      watchedAt={null}
      now={now}
      pending={false}
      onConfirm={onConfirm}
      onCancel={() => undefined}
    />,
  )

  const input = screen.getByLabelText('S02E03 观看时间')
  expect(input).toHaveAttribute('type', 'datetime-local')
  expect(input).toHaveAttribute('step', '1')
  expect(input).toHaveAttribute('max', '2026-07-14T17:08:09')
  fireEvent.change(input, { target: { value: '2026-07-14T17:08:10' } })
  await user.click(screen.getByRole('button', { name: '确定 S02E03 观看时间' }))

  expect(await screen.findByRole('alert')).toHaveTextContent('观看时间不能晚于当前时间')
  expect(input).toHaveValue('2026-07-14T17:08:10.000')
  expect(onConfirm).not.toHaveBeenCalled()
})

it('cancels an edit and restores focus to its time button', async () => {
  const user = userEvent.setup()

  function Harness() {
    const [editing, setEditing] = useState(true)
    const triggerRef = useRef<HTMLButtonElement>(null)
    return (
      <>
        <button ref={triggerRef} type="button" onClick={() => setEditing(true)}>设置 S01E01 观看时间</button>
        {editing ? (
          <EpisodeTimeEditor
            episodeLabel="S01E01"
            watchedAt="2026-07-13T12:00:01Z"
            now={now}
            pending={false}
            returnFocusRef={triggerRef}
            onConfirm={() => undefined}
            onCancel={() => setEditing(false)}
          />
        ) : null}
      </>
    )
  }

  renderWithQueryClient(<Harness />)
  fireEvent.change(screen.getByLabelText('S01E01 观看时间'), { target: { value: '2026-07-12T11:00:00' } })
  await user.click(screen.getByRole('button', { name: '取消 S01E01 观看时间' }))

  expect(screen.queryByLabelText('S01E01 观看时间')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: '设置 S01E01 观看时间' })).toHaveFocus()
})
