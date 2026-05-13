import { beforeEach, describe, expect, it } from 'vitest'

import { consumeSSOStartIntent, setSSOStartIntent } from './sso-start-intent.js'

describe('SSO start intent', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('consumes a matching provider once', () => {
    setSSOStartIntent('google', '/login')

    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: true,
      returnTo: '/login',
    })
    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: false,
      returnTo: '/login',
    })
  })

  it('rejects and clears mismatched providers', () => {
    setSSOStartIntent('google', '/sessions')

    expect(consumeSSOStartIntent('github')).toEqual({
      authorized: false,
      returnTo: '/sessions',
    })
    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: false,
      returnTo: '/sessions',
    })
  })

  it('returns to the launch route after the provider redirect was consumed', () => {
    setSSOStartIntent('google', '/u/4/plan')
    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: true,
      returnTo: '/u/4/plan',
    })

    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: false,
      returnTo: '/u/4/plan',
    })
  })

  it('does not restore executable SSO routes as return targets', () => {
    setSSOStartIntent('google', '/auth/sso/google')

    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: true,
      returnTo: '/login',
    })
    expect(consumeSSOStartIntent('google')).toEqual({
      authorized: false,
      returnTo: '/login',
    })
  })
})
