import { describe, it, expect } from 'vitest'
import {
  encodeUint32Le,
  decodeUint32Le,
  prependPacketLen,
  compareUint8Arrays,
} from './binary.js'

describe('encodeUint32Le', () => {
  it('should encode 0 as [0, 0, 0, 0]', () => {
    const result = encodeUint32Le(0)
    expect(result).toEqual(new Uint8Array([0, 0, 0, 0]))
  })

  it('should encode 1 as [1, 0, 0, 0]', () => {
    const result = encodeUint32Le(1)
    expect(result).toEqual(new Uint8Array([1, 0, 0, 0]))
  })

  it('should encode 255 as [255, 0, 0, 0]', () => {
    const result = encodeUint32Le(255)
    expect(result).toEqual(new Uint8Array([255, 0, 0, 0]))
  })

  it('should encode 256 as [0, 1, 0, 0]', () => {
    const result = encodeUint32Le(256)
    expect(result).toEqual(new Uint8Array([0, 1, 0, 0]))
  })

  it('should encode 65535 as [255, 255, 0, 0]', () => {
    const result = encodeUint32Le(65535)
    expect(result).toEqual(new Uint8Array([255, 255, 0, 0]))
  })

  it('should encode 16777215 as [255, 255, 255, 0]', () => {
    const result = encodeUint32Le(16777215)
    expect(result).toEqual(new Uint8Array([255, 255, 255, 0]))
  })

  it('should encode max uint32 (4294967295) as [255, 255, 255, 255]', () => {
    const result = encodeUint32Le(4294967295)
    expect(result).toEqual(new Uint8Array([255, 255, 255, 255]))
  })

  it('should encode 1000 correctly', () => {
    const result = encodeUint32Le(1000)
    expect(result).toEqual(new Uint8Array([232, 3, 0, 0]))
  })
})

describe('decodeUint32Le', () => {
  it('should decode [0, 0, 0, 0] as 0', () => {
    const result = decodeUint32Le(new Uint8Array([0, 0, 0, 0]))
    expect(result).toBe(0)
  })

  it('should decode [1, 0, 0, 0] as 1', () => {
    const result = decodeUint32Le(new Uint8Array([1, 0, 0, 0]))
    expect(result).toBe(1)
  })

  it('should decode [255, 0, 0, 0] as 255', () => {
    const result = decodeUint32Le(new Uint8Array([255, 0, 0, 0]))
    expect(result).toBe(255)
  })

  it('should decode [0, 1, 0, 0] as 256', () => {
    const result = decodeUint32Le(new Uint8Array([0, 1, 0, 0]))
    expect(result).toBe(256)
  })

  it('should decode [255, 255, 0, 0] as 65535', () => {
    const result = decodeUint32Le(new Uint8Array([255, 255, 0, 0]))
    expect(result).toBe(65535)
  })

  it('should decode [255, 255, 255, 0] as 16777215', () => {
    const result = decodeUint32Le(new Uint8Array([255, 255, 255, 0]))
    expect(result).toBe(16777215)
  })

  it('should decode [255, 255, 255, 255] as 4294967295', () => {
    const result = decodeUint32Le(new Uint8Array([255, 255, 255, 255]))
    expect(result).toBe(4294967295)
  })

  it('should decode [232, 3, 0, 0] as 1000', () => {
    const result = decodeUint32Le(new Uint8Array([232, 3, 0, 0]))
    expect(result).toBe(1000)
  })

  it('should handle arrays shorter than 4 bytes', () => {
    const result = decodeUint32Le(new Uint8Array([1, 2]))
    expect(result).toBe(513) // 1 + 2*256
  })

  it('should handle empty array', () => {
    const result = decodeUint32Le(new Uint8Array([]))
    expect(result).toBe(0)
  })
})

describe('encode and decode round-trip', () => {
  const testValues = [
    0, 1, 255, 256, 1000, 65535, 65536, 16777215, 16777216, 4294967295,
  ]

  testValues.forEach((value) => {
    it(`should correctly round-trip ${value}`, () => {
      const encoded = encodeUint32Le(value)
      const decoded = decodeUint32Le(encoded)
      expect(decoded).toBe(value)
    })
  })
})

describe('prependPacketLen', () => {
  it('should prepend length to empty array', () => {
    const data = new Uint8Array([])
    const result = prependPacketLen(data)
    expect(result).toEqual(new Uint8Array([0, 0, 0, 0]))
  })

  it('should prepend length to single byte array', () => {
    const data = new Uint8Array([42])
    const result = prependPacketLen(data)
    expect(result).toEqual(new Uint8Array([1, 0, 0, 0, 42]))
  })

  it('should prepend length to multi-byte array', () => {
    const data = new Uint8Array([1, 2, 3, 4, 5])
    const result = prependPacketLen(data)
    expect(result).toEqual(new Uint8Array([5, 0, 0, 0, 1, 2, 3, 4, 5]))
  })

  it('should correctly prepend length of 256 bytes', () => {
    const data = new Uint8Array(256).fill(0)
    const result = prependPacketLen(data)
    expect(result.length).toBe(260) // 4 bytes for length + 256 bytes data
    expect(result[0]).toBe(0) // 256 in little-endian: [0, 1, 0, 0]
    expect(result[1]).toBe(1)
    expect(result[2]).toBe(0)
    expect(result[3]).toBe(0)
  })

  it('should preserve original data', () => {
    const data = new Uint8Array([10, 20, 30])
    const result = prependPacketLen(data)
    expect(result.slice(4)).toEqual(data)
  })
})

describe('compareUint8Arrays', () => {
  it('should return true for identical references', () => {
    const arr = new Uint8Array([1, 2, 3])
    expect(compareUint8Arrays(arr, arr)).toBe(true)
  })

  it('should return true for equal arrays', () => {
    const a = new Uint8Array([1, 2, 3])
    const b = new Uint8Array([1, 2, 3])
    expect(compareUint8Arrays(a, b)).toBe(true)
  })

  it('should return true for empty arrays', () => {
    const a = new Uint8Array([])
    const b = new Uint8Array([])
    expect(compareUint8Arrays(a, b)).toBe(true)
  })

  it('should return false for arrays of different lengths', () => {
    const a = new Uint8Array([1, 2, 3])
    const b = new Uint8Array([1, 2])
    expect(compareUint8Arrays(a, b)).toBe(false)
  })

  it('should return false for arrays with different values', () => {
    const a = new Uint8Array([1, 2, 3])
    const b = new Uint8Array([1, 2, 4])
    expect(compareUint8Arrays(a, b)).toBe(false)
  })

  it('should return false for arrays with different values at start', () => {
    const a = new Uint8Array([1, 2, 3])
    const b = new Uint8Array([0, 2, 3])
    expect(compareUint8Arrays(a, b)).toBe(false)
  })

  it('should return false for arrays with different values in middle', () => {
    const a = new Uint8Array([1, 2, 3])
    const b = new Uint8Array([1, 0, 3])
    expect(compareUint8Arrays(a, b)).toBe(false)
  })

  it('should return false when first array is null/undefined', () => {
    const b = new Uint8Array([1, 2, 3])
    expect(compareUint8Arrays(null, b)).toBe(false)
    expect(compareUint8Arrays(undefined, b)).toBe(false)
  })

  it('should return false when second array is null/undefined', () => {
    const a = new Uint8Array([1, 2, 3])
    expect(compareUint8Arrays(a, null)).toBe(false)
    expect(compareUint8Arrays(a, undefined)).toBe(false)
  })

  it('should return true when both arrays are the same null/undefined reference', () => {
    // Identity check happens first, so null === null and undefined === undefined returns true
    expect(compareUint8Arrays(null, null)).toBe(true)
    expect(compareUint8Arrays(undefined, undefined)).toBe(true)
  })

  it('should return false when comparing null and undefined', () => {
    expect(compareUint8Arrays(null, undefined)).toBe(false)
  })

  it('should handle large arrays', () => {
    const size = 10000
    const a = new Uint8Array(size).fill(1)
    const b = new Uint8Array(size).fill(1)
    const c = new Uint8Array(size).fill(2)

    expect(compareUint8Arrays(a, b)).toBe(true)
    expect(compareUint8Arrays(a, c)).toBe(false)
  })

  it('should detect difference at last element', () => {
    const a = new Uint8Array([1, 2, 3, 4, 5])
    const b = new Uint8Array([1, 2, 3, 4, 6])
    expect(compareUint8Arrays(a, b)).toBe(false)
  })
})
