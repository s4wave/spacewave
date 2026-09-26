import { describe, expect, it } from 'vitest'

import { safeHref } from './safe-href.js'

describe('safeHref', () => {
  it('passes through http and https URLs', () => {
    expect(safeHref('https://github.com/paralin')).toBe(
      'https://github.com/paralin',
    )
    expect(safeHref('http://example.com')).toBe('http://example.com')
  })

  it('passes through mailto and tel URLs', () => {
    expect(safeHref('mailto:hi@example.com')).toBe('mailto:hi@example.com')
    expect(safeHref('tel:+15551234567')).toBe('tel:+15551234567')
  })

  it('passes through relative URLs', () => {
    expect(safeHref('/blog/2026/04/post')).toBe('/blog/2026/04/post')
    expect(safeHref('./post')).toBe('./post')
    expect(safeHref('post')).toBe('post')
  })

  it('rejects javascript: and other dangerous schemes', () => {
    expect(safeHref('javascript:alert(1)')).toBe('#')
    expect(safeHref('JaVaScRiPt:alert(1)')).toBe('#')
    expect(safeHref('  javascript:alert(1)')).toBe('#')
    expect(safeHref('data:text/html,<script>alert(1)</script>')).toBe('#')
    expect(safeHref('vbscript:msgbox(1)')).toBe('#')
    expect(safeHref('file:///etc/passwd')).toBe('#')
  })

  it('rejects dangerous schemes obscured by characters the URL parser strips', () => {
    expect(safeHref('java\tscript:alert(1)')).toBe('#')
    expect(safeHref('java\nscript:alert(1)')).toBe('#')
    expect(safeHref('java\rscript:alert(1)')).toBe('#')
    expect(safeHref('javascript\t:alert(1)')).toBe('#')
    expect(safeHref('\x01javascript:alert(1)')).toBe('#')
    expect(safeHref('\x00 javascript:alert(1)')).toBe('#')
    expect(safeHref('\x1f\tdata:text/html,x')).toBe('#')
  })

  it('returns the normalized URL for safe inputs', () => {
    expect(safeHref('ht\ttps://example.com')).toBe('https://example.com')
    expect(safeHref('\x01/blog/post')).toBe('/blog/post')
  })

  it('returns # for empty or undefined', () => {
    expect(safeHref(undefined)).toBe('#')
    expect(safeHref('')).toBe('#')
    expect(safeHref('   ')).toBe('#')
  })
})
