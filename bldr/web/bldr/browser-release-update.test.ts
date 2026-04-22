import { describe, expect, it } from 'vitest'

import { shouldReloadForPromotedGeneration } from './browser-release-update.js'

describe('browser release update reload policy', () => {
  it('does not reload when the promoted generation matches the active shell', () => {
    expect(
      shouldReloadForPromotedGeneration('deadbeefcafebabe', 'deadbeefcafebabe'),
    ).toBe(false)
  })

  it('reloads when the promoted generation changes', () => {
    expect(
      shouldReloadForPromotedGeneration('deadbeefcafebabe', 'feedfacecafed00d'),
    ).toBe(true)
  })

  it('ignores empty promotion messages', () => {
    expect(shouldReloadForPromotedGeneration('deadbeefcafebabe', undefined)).toBe(
      false,
    )
  })
})
