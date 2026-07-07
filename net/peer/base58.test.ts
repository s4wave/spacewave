import { describe, expect, it } from 'vitest'

import { base58Decode, base58Encode } from './base58.js'

describe('base58', () => {
  it('matches bitcoin alphabet vectors used by peer id decoding', () => {
    expect(base58Encode(new Uint8Array())).toBe('')
    expect(base58Encode(new TextEncoder().encode('hello world'))).toBe(
      'StV1DL6CwTryKyV',
    )
    expect(base58Encode(new Uint8Array([0, 0, 1, 2, 3]))).toBe('11Ldp')
    expect(base58Encode(new Uint8Array(32).fill(1))).toBe(
      '4vJ9JU1bJJE96FWSJKvHsmmFADCg4gpZQff4P3bkLKi',
    )
  })

  it('round-trips bytes with leading zeroes', () => {
    const bytes = new Uint8Array(32)
    for (let i = 2; i < bytes.length; i++) {
      bytes[i] = i
    }

    expect(base58Decode(base58Encode(bytes))).toEqual(bytes)
  })

  it('rejects characters outside the bitcoin alphabet', () => {
    expect(base58Decode('0OIl')).toBeNull()
  })
})
