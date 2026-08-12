import { beforeEach, describe, expect, it } from 'vitest'

import {
  clearSSOBrowserBinding,
  getSSOBrowserBinding,
  consumeSSOStartIntent,
  setSSOBrowserBinding,
  setSSOStartIntent,
} from './sso-start-intent.js'

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

  it('does not authorize malformed persisted intents', () => {
    sessionStorage.setItem(
      'spacewave-sso-start-provider',
      JSON.stringify({ provider: 'google', returnTo: 7 }),
    )

    expect(consumeSSOStartIntent('google').authorized).toBe(false)
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

  it('retains the browser verifier until exchange succeeds', () => {
    const binding = {
      verifier: 'verifier',
      verifierHash: 'commitment',
      devicePublicKey: 'public-key',
      devicePrivateKey: 'private-key',
    }
    setSSOBrowserBinding(binding)

    expect(getSSOBrowserBinding()).toEqual(binding)
    expect(getSSOBrowserBinding()).toEqual(binding)
    clearSSOBrowserBinding()
    expect(getSSOBrowserBinding()).toBeNull()
  })

  it('treats inherited same-origin session storage as browser-flow state', () => {
    const binding = {
      verifier: 'inherited-verifier',
      verifierHash: 'inherited-commitment',
      devicePublicKey: 'inherited-public-key',
      devicePrivateKey: 'inherited-private-key',
    }
    sessionStorage.setItem(
      'spacewave-sso-browser-binding',
      JSON.stringify(binding),
    )

    expect(getSSOBrowserBinding()).toEqual(binding)
  })
})
