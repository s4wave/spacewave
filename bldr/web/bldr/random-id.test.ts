import { describe, expect, it, vi } from 'vitest'

import { randomId } from './random-id.js'

describe('randomId', () => {
  it('uses browser crypto instead of Math.random', () => {
    const mathRandom = vi.spyOn(Math, 'random')

    const id = randomId()

    expect(id).toMatch(/^[0-9a-f]{16}$/)
    expect(mathRandom).not.toHaveBeenCalled()
  })
})
