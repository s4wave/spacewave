import { afterEach, describe, expect, it } from 'vitest'

import { FALLBACK_APP_BUILD_INFO, getAppBuildInfo } from './build-info.js'

describe('getAppBuildInfo', () => {
  afterEach(() => {
    globalThis.__BLDR_BUILD_INFO__ = undefined
    globalThis.__swGenerationId = undefined
  })

  it('falls back cleanly when no build globals exist', () => {
    expect(getAppBuildInfo()).toEqual(FALLBACK_APP_BUILD_INFO)
  })

  it('includes the active browser generation id when bootstrap exposes it', () => {
    globalThis.__BLDR_BUILD_INFO__ = {
      version: '1.2.3',
      goVersion: 'go1.25',
      goos: 'js',
      goarch: 'wasm',
    }
    globalThis.__swGenerationId = 'deadbeefcafebabe'

    expect(getAppBuildInfo()).toMatchObject({
      version: '1.2.3',
      browserGenerationId: 'deadbeefcafebabe',
    })
  })

  it('uses mainVersion for release labels when raw version is absent', () => {
    globalThis.__BLDR_BUILD_INFO__ = {
      mainVersion: '2026.7.8',
      goVersion: 'go1.25',
      goos: 'js',
      goarch: 'wasm',
    }

    const info = getAppBuildInfo()
    expect(info.version).toBe('2026.7.8')
    expect(info.cornerLabel).toBe('2026.7.8@go1.25')
    expect(info.cornerLabel).not.toContain('dev')
  })
})
