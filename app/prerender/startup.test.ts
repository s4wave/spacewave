import { describe, expect, it } from 'vitest'

import { shouldShowStartupLoading } from './startup.js'

describe('startup loading route selection', () => {
  it('shows landing for a new visitor on bare root', () => {
    expect(shouldShowStartupLoading('/', '', false)).toBe(false)
  })

  it('shows loading for hash routes on root', () => {
    expect(shouldShowStartupLoading('/', '#/login', false)).toBe(true)
  })

  it('shows loading for direct app pathnames', () => {
    expect(shouldShowStartupLoading('/login', '', false)).toBe(true)
  })

  it('shows loading after prior interaction', () => {
    expect(shouldShowStartupLoading('/', '', true)).toBe(true)
  })

  it('keeps static pathnames eligible for prerendered startup content', () => {
    expect(shouldShowStartupLoading('/pricing', '', false)).toBe(false)
  })
})
