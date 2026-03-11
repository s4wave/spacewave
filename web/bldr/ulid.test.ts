import { describe, expect, it } from 'vitest'

import { newULID, parseULID } from './ulid.js'

describe('newULID', () => {
  it('returns a 26-character lowercase string', () => {
    const id = newULID()
    expect(id).toHaveLength(26)
    expect(id).toBe(id.toLowerCase())
  })

  it('generates unique values', () => {
    const a = newULID()
    const b = newULID()
    expect(a).not.toBe(b)
  })
})

describe('parseULID', () => {
  it('accepts a valid lowercase ULID', () => {
    const id = newULID()
    expect(parseULID(id)).toBe(id)
  })

  it('rejects uppercase ULID', () => {
    const id = newULID().toUpperCase()
    expect(() => parseULID(id)).toThrow('invalid ulid')
  })

  it('rejects mixed case ULID', () => {
    const id = newULID()
    // Find a letter character to uppercase (skip leading digits)
    let mixed = id
    for (let i = 0; i < id.length; i++) {
      if (id[i]! >= 'a' && id[i]! <= 'z') {
        mixed = id.slice(0, i) + id[i]!.toUpperCase() + id.slice(i + 1)
        break
      }
    }
    expect(mixed).not.toBe(id)
    expect(() => parseULID(mixed)).toThrow('invalid ulid')
  })

  it('rejects wrong length', () => {
    expect(() => parseULID('abc')).toThrow('invalid ulid')
    expect(() => parseULID('')).toThrow('invalid ulid')
  })

  it('rejects ULID with timestamp before Nov 2009', () => {
    // All zeros = timestamp 0
    const ancient = '00000000000000000000000000'
    expect(() => parseULID(ancient)).toThrow('invalid ulid')
  })
})
