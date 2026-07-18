import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@fontsource-variable/outfit/index.css'

import { App } from './app/App'
import { applyThemePreference, readThemePreference } from './lib/theme'
import './styles/tokens.css'
import './styles/global.css'

applyThemePreference(readThemePreference())

const root = document.getElementById('root')

if (!root) {
  throw new Error('Application root is missing')
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
