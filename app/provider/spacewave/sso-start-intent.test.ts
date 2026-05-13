import { beforeEach, describe, expect, it } from 'vitest'

import { consumeSSOStartIntent, setSSOStartIntent } from './sso-start-intent.js'

describe('SSO start intent', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('consumes a matching provider once', () => {
    setSSOStartIntent('google')

    expect(consumeSSOStartIntent('google')).toBe(true)
    expect(consumeSSOStartIntent('google')).toBe(false)
  })

  it('rejects and clears mismatched providers', () => {
    setSSOStartIntent('google')

    expect(consumeSSOStartIntent('github')).toBe(false)
    expect(consumeSSOStartIntent('google')).toBe(false)
  })
})
