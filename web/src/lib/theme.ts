export type ThemePreference = 'system' | 'light' | 'dark'

export const themeStorageKey = 'video-record.theme'

export function readThemePreference(): ThemePreference {
  try {
    const value = window.localStorage.getItem(themeStorageKey)
    if (value === 'light' || value === 'dark' || value === 'system') return value
  } catch {
    // ignore storage failures
  }
  return 'system'
}

export function writeThemePreference(preference: ThemePreference) {
  try {
    window.localStorage.setItem(themeStorageKey, preference)
  } catch {
    // ignore storage failures
  }
}

export function applyThemePreference(preference: ThemePreference) {
  const root = document.documentElement
  if (preference === 'system') {
    root.removeAttribute('data-theme')
    return
  }
  root.setAttribute('data-theme', preference)
}

export function resolveThemePreference(preference: ThemePreference, prefersDark: boolean): 'light' | 'dark' {
  if (preference === 'system') return prefersDark ? 'dark' : 'light'
  return preference
}
