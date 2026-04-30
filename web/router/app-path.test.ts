import { afterEach, describe, expect, it } from 'vitest'

import { getAppPath, normalizeAppPath } from './app-path.js'

describe('app path helpers', () => {
  afterEach(() => {
    window.location.hash = ''
    window.history.replaceState({}, '', '/')
  })

  it('normalizes encoded hash paths back to decoded route paths', () => {
    window.location.hash =
      '#/u/1/so/space/-/files/-/test/dir/video%20with%20spaces.mp4'

    expect(getAppPath()).toBe(
      '/u/1/so/space/-/files/-/test/dir/video with spaces.mp4',
    )
  })

  it('strips query params before decoding', () => {
    expect(normalizeAppPath('/recover%20flow?code=abc')).toBe('/recover flow')
  })

  it('uses direct login pathnames as app routes', () => {
    window.history.replaceState({}, '', '/login')

    expect(getAppPath()).toBe('/login')
  })

  it('preserves literal percent characters in already-decoded paths', () => {
    expect(normalizeAppPath('/u/1/notes/100% ready.txt')).toBe(
      '/u/1/notes/100% ready.txt',
    )
  })
})
