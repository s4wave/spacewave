import { describe, expect, it } from 'vitest'

import {
  bytesToHex,
  bytesToText,
  detectMode,
  hexToBytes,
  parseValue,
  renderValue,
} from './kv-encoding.js'

const encode = (text: string) => new TextEncoder().encode(text)

describe('kv-encoding', () => {
  it('round-trips text values', () => {
    const bytes = parseValue('hello', 'text')
    expect(bytesToText(bytes)).toBe('hello')
    expect(renderValue(bytes, 'text')).toBe('hello')
  })

  it('renders and parses hex with spaces and 0x prefixes', () => {
    expect(bytesToHex(new Uint8Array([0, 255, 16]))).toBe('00 ff 10')
    expect([...hexToBytes('00 ff 0x10')]).toEqual([0, 255, 16])
    expect(() => hexToBytes('abc')).toThrow()
    expect(() => hexToBytes('zz')).toThrow()
  })

  it('pretty-prints and parses JSON', () => {
    const bytes = encode('{"a":1}')
    expect(renderValue(bytes, 'json')).toBe('{\n  "a": 1\n}')
    expect(bytesToText(parseValue('{ "a": 1 }', 'json'))).toBe('{"a":1}')
    expect(() => parseValue('not json', 'json')).toThrow()
  })

  it('auto-detects the best display mode', () => {
    expect(detectMode(encode('{"a":1}'))).toBe('json')
    expect(detectMode(encode('plain text'))).toBe('text')
    expect(detectMode(new Uint8Array([0x00, 0x01, 0xff]))).toBe('hex')
    expect(detectMode(new Uint8Array())).toBe('text')
    expect(detectMode(encode('{ not really json'))).toBe('text')
  })
})
