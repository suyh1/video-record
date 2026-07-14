import { describe, expect, it } from 'vitest'

// @ts-expect-error The Node runner helper is JavaScript by design.
import { playwrightEnvironment } from '../../scripts/e2e-environment.mjs'

describe('playwrightEnvironment', () => {
  it('removes NO_COLOR before Playwright adds FORCE_COLOR', () => {
    const source = { NO_COLOR: '1', PATH: '/synthetic/bin' }

    expect(playwrightEnvironment(source)).toEqual({ PATH: '/synthetic/bin' })
    expect(source).toEqual({ NO_COLOR: '1', PATH: '/synthetic/bin' })
  })
})
