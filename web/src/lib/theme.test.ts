import { afterEach, describe, expect, it } from 'vitest'

import {
  applyThemePreference,
  readThemePreference,
  resolveThemePreference,
  themeStorageKey,
  writeThemePreference,
} from './theme'

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
  window.localStorage.removeItem(themeStorageKey)
})

describe('theme preference', () => {
  it('defaults to system and persists light or dark choices', () => {
    expect(readThemePreference()).toBe('system')
    writeThemePreference('dark')
    expect(readThemePreference()).toBe('dark')
    applyThemePreference('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    applyThemePreference('system')
    expect(document.documentElement.hasAttribute('data-theme')).toBe(false)
  })

  it('resolves system preference from prefers-color-scheme', () => {
    expect(resolveThemePreference('system', true)).toBe('dark')
    expect(resolveThemePreference('system', false)).toBe('light')
    expect(resolveThemePreference('light', true)).toBe('light')
  })
})
