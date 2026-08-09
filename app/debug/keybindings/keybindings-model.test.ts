import { describe, expect, it } from 'vitest'

import { contextsOverlap } from './keybindings-model.js'

describe('contextsOverlap', () => {
  it('treats Global overlap symmetrically', () => {
    expect(contextsOverlap('Global', 'Editor')).toBe(true)
    expect(contextsOverlap('Editor', 'Global')).toBe(true)
    expect(contextsOverlap('Editor', 'Canvas')).toBe(false)
  })
})
