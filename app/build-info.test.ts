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
})
