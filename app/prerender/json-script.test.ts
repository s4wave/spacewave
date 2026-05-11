import { describe, expect, it } from 'vitest'

import { serializeJsonScriptData } from './json-script.js'

describe('serializeJsonScriptData', () => {
  it('escapes values that can break out of inline script tags', () => {
    const lineSeparator = String.fromCharCode(0x2028)
    const paragraphSeparator = String.fromCharCode(0x2029)
    const value = {
      html: '</script><img src=x onerror=alert(1)>',
      ampersand: 'a&b',
      separators: `a${lineSeparator}b${paragraphSeparator}c`,
    }

    const serialized = serializeJsonScriptData(value)

    expect(serialized).not.toContain('</script')
    expect(serialized).not.toContain('<img')
    expect(serialized).not.toContain('a&b')
    expect(serialized).toContain('\\u003c/script\\u003e')
    expect(serialized).toContain('\\u0026')
    expect(serialized).toContain('\\u2028')
    expect(serialized).toContain('\\u2029')
    expect(JSON.parse(serialized)).toEqual(value)
  })
})
