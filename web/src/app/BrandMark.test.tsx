import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { BrandMark } from './BrandMark'

describe('BrandMark', () => {
  it('renders the decorative film archive symbol at the requested size', () => {
    render(<BrandMark data-testid="brand-mark" size={28} />)

    const mark = screen.getByTestId('brand-mark')
    expect(mark).toHaveAttribute('data-brand-mark', 'film-archive')
    expect(mark).toHaveAttribute('aria-hidden', 'true')
    expect(mark).toHaveAttribute('width', '28')
    expect(mark).toHaveAttribute('height', '28')
  })
})
