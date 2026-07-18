import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'

import { RatingPicker } from './RatingPicker'

describe('RatingPicker', () => {
  it('selects 8.5 via the scale and exposes aria-valuenow', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('')
      return <RatingPicker value={value} onChange={setValue} />
    }

    render(<Harness />)
    const slider = screen.getByRole('slider', { name: '评分' })
    expect(slider).not.toHaveAttribute('aria-valuenow')

    await user.click(screen.getByRole('button', { name: '评分 8.5' }))
    expect(slider).toHaveAttribute('aria-valuenow', '8.5')
    expect(slider).toHaveAttribute('aria-valuetext', '8.5 / 10')
    expect(screen.getByLabelText('精确评分')).toHaveValue(8.5)
  })

  it('supports keyboard nudging in 0.5 steps and clearing', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('8')
      return <RatingPicker value={value} onChange={setValue} />
    }

    render(<Harness />)
    const slider = screen.getByRole('slider', { name: '评分' })
    slider.focus()
    await user.keyboard('{ArrowRight}')
    expect(slider).toHaveAttribute('aria-valuenow', '8.5')
    await user.keyboard('{ArrowLeft}{ArrowLeft}')
    expect(slider).toHaveAttribute('aria-valuenow', '7.5')
    await user.keyboard('{Delete}')
    expect(slider).not.toHaveAttribute('aria-valuenow')
    expect(screen.getByLabelText('精确评分')).toHaveValue(null)
  })

  it('keeps the precise number input as a secondary entry', async () => {
    const user = userEvent.setup()

    function Harness() {
      const [value, setValue] = useState('')
      return <RatingPicker value={value} onChange={setValue} />
    }

    render(<Harness />)
    const input = screen.getByLabelText('精确评分')
    await user.clear(input)
    await user.type(input, '9.5')
    expect(screen.getByRole('slider', { name: '评分' })).toHaveAttribute('aria-valuenow', '9.5')
  })
})
