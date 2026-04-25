import { describe, expect, it } from 'vitest'

import { canRenameSpace } from './permissions.js'

describe('canRenameSpace', () => {
  it('allows rename for local spaces', () => {
    expect(canRenameSpace('local', false)).toBe(true)
  })

  it('allows rename for cloud spaces with manage permission', () => {
    expect(canRenameSpace('spacewave', true)).toBe(true)
  })

  it('blocks rename for cloud viewers without manage permission', () => {
    expect(canRenameSpace('spacewave', false)).toBe(false)
  })

  it('blocks rename for unsupported providers', () => {
    expect(canRenameSpace('other', true)).toBe(false)
  })
})
