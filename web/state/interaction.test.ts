import { describe, it, expect, beforeEach } from 'vitest'
import {
  hasInteracted,
  markInteracted,
  clearInteracted,
} from './interaction.js'

describe('interaction', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('returns false when not interacted', () => {
    expect(hasInteracted()).toBe(false)
  })

  it('returns true after markInteracted', () => {
    markInteracted()
    expect(hasInteracted()).toBe(true)
  })

  it('returns false after clearInteracted', () => {
    markInteracted()
    expect(hasInteracted()).toBe(true)
    clearInteracted()
    expect(hasInteracted()).toBe(false)
  })

  it('persists across function calls', () => {
    markInteracted()
    expect(hasInteracted()).toBe(true)
    expect(hasInteracted()).toBe(true)
  })
})
