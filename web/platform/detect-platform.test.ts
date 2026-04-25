import { describe, expect, it } from 'vitest'

import { detectPlatform } from './detect-platform.js'

function fakeNav(
  userAgent: string,
  userAgentData?: { platform: string },
): Navigator {
  return { userAgent, userAgentData } as unknown as Navigator
}

describe('detectPlatform', () => {
  it('detects Chrome on Apple Silicon macOS via userAgentData', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36',
      { platform: 'macOS' },
    )
    expect(detectPlatform(nav)).toEqual({ os: 'macos', arch: 'arm64' })
  })

  it('detects Chrome on Intel macOS (defaults to arm64 since UA is spoofed)', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36',
      { platform: 'macOS' },
    )
    expect(detectPlatform(nav)).toEqual({ os: 'macos', arch: 'arm64' })
  })

  it('detects Firefox on macOS via userAgent regex (no userAgentData)', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:125.0) Gecko/20100101 Firefox/125.0',
    )
    expect(detectPlatform(nav)).toEqual({ os: 'macos', arch: 'arm64' })
  })

  it('detects Edge on Windows x86_64', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0',
      { platform: 'Windows' },
    )
    expect(detectPlatform(nav)).toEqual({ os: 'windows', arch: 'amd64' })
  })

  it('detects Edge on Windows ARM64 when UA advertises it', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (Windows NT 10.0; ARM64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0',
      { platform: 'Windows' },
    )
    expect(detectPlatform(nav)).toEqual({ os: 'windows', arch: 'arm64' })
  })

  it('detects Chrome on Linux x86_64', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36',
      { platform: 'Linux' },
    )
    expect(detectPlatform(nav)).toEqual({ os: 'linux', arch: 'amd64' })
  })

  it('detects Firefox on Linux aarch64', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (X11; Linux aarch64; rv:125.0) Gecko/20100101 Firefox/125.0',
    )
    expect(detectPlatform(nav)).toEqual({ os: 'linux', arch: 'arm64' })
  })

  it('excludes Android Linux UAs', () => {
    const nav = fakeNav(
      'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Mobile Safari/537.36',
    )
    expect(detectPlatform(nav)).toBeNull()
  })

  it('returns null for a gibberish user agent', () => {
    const nav = fakeNav('Gibberish/1.0 (Unknown Platform)')
    expect(detectPlatform(nav)).toBeNull()
  })
})
