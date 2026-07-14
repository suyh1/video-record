import { describe, expect, it } from 'vitest'

import {
  formatLocalSeconds,
  fromDateTimeLocalValue,
  isFutureDateTimeLocalValue,
  toDateTimeLocalValue,
} from './dateTime'

describe('local second-precision date time helpers', () => {
  it('formats an ISO instant in the requested display timezone', () => {
    expect(formatLocalSeconds('2026-07-14T12:30:45Z', 'UTC')).toBe('2026-07-14 12:30:45')
  })

  it('round trips native datetime-local values without slicing UTC strings', () => {
    const date = new Date(2026, 6, 14, 12, 30, 45)
    const value = toDateTimeLocalValue(date)
    expect(value).toMatch(/^2026-07-14T12:30:45$/)
    expect(fromDateTimeLocalValue(value)?.getTime()).toBe(date.getTime())
    expect(fromDateTimeLocalValue('')).toBeNull()
  })

  it('compares local input values against the supplied current instant', () => {
    const now = new Date(2026, 6, 14, 12, 30, 45)
    expect(isFutureDateTimeLocalValue('2026-07-14T12:30:46', now)).toBe(true)
    expect(isFutureDateTimeLocalValue('2026-07-14T12:30:45', now)).toBe(false)
  })
})
