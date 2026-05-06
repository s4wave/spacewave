import { describe, expect, it } from 'vitest'

import { buildInviteLink, SPACEWAVE_PUBLIC_BASE_URL } from './urls.js'

describe('buildInviteLink', () => {
  it('uses the canonical hosted origin when no public base url is provided', () => {
    expect(buildInviteLink(undefined, 'abc123')).toBe(
      `${SPACEWAVE_PUBLIC_BASE_URL}/#/join/abc123`,
    )
  })

  it('prefers the configured cloud public base url', () => {
    expect(buildInviteLink('https://staging.spacewave.app', 'enc')).toBe(
      'https://staging.spacewave.app/#/join/enc',
    )
  })

  it('strips trailing slashes from the base url', () => {
    expect(buildInviteLink('https://spacewave.app///', 'tok')).toBe(
      'https://spacewave.app/#/join/tok',
    )
  })

  it('falls back to the canonical origin for empty strings', () => {
    expect(buildInviteLink('', 'tok')).toBe(
      `${SPACEWAVE_PUBLIC_BASE_URL}/#/join/tok`,
    )
  })

  it('never returns the desktop app:// origin', () => {
    const link = buildInviteLink(undefined, 'tok')
    expect(link.startsWith('app://')).toBe(false)
    expect(link.startsWith('https://')).toBe(true)
  })
})
