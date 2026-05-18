import { afterEach, describe, expect, it } from 'vitest'

import {
  clearDebugContext,
  getDebugContext,
  setDebugContext,
  type DebugContext,
} from './context.js'

describe('debug context', () => {
  afterEach(() => {
    clearDebugContext()
  })

  it('clears only the matching debug context generation', () => {
    const first: DebugContext = { generation: 1 }
    const second: DebugContext = { generation: 2 }

    setDebugContext(first)
    setDebugContext(second)
    clearDebugContext(first)

    expect(getDebugContext()).toBe(second)

    clearDebugContext(second)

    expect(() => getDebugContext()).toThrow('Debug context not initialized')
  })
})
