import { afterEach, describe, expect, it } from 'vitest'

import {
  getAppPath,
  isPathnameAppRoute,
  normalizeAppPath,
  setAppPath,
} from './app-path.js'

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

  it('normalizes hash-prefixed paths', () => {
    expect(normalizeAppPath('#/quickstart/drive')).toBe('/quickstart/drive')
  })

  it('uses direct login pathnames as app routes', () => {
    window.history.replaceState({}, '', '/login')

    expect(getAppPath()).toBe('/login')
  })

  it('keeps display query pathnames as app routes after OTP hash wipe', () => {
    window.history.replaceState(
      {},
      '',
      '/display?path=docs%2Fhello&component=viewer.markdown',
    )

    expect(getAppPath()).toBe('/display')
  })

  it('allowlists display child pathnames as app routes', () => {
    expect(isPathnameAppRoute('/display/child')).toBe(true)
  })

  it('preserves literal percent characters in already-decoded paths', () => {
    expect(normalizeAppPath('/u/1/notes/100% ready.txt')).toBe(
      '/u/1/notes/100% ready.txt',
    )
  })

  it('sets app paths as root hash routes from static pathnames', () => {
    window.history.replaceState({}, '', '/quickstart/drive')

    setAppPath('#/quickstart/drive')

    expect(window.location.pathname).toBe('/')
    expect(window.location.hash).toBe('#/quickstart/drive')
  })

  it('canonicalizes app-only routes from static pathnames', () => {
    window.history.replaceState({}, '', '/pricing')

    setAppPath('/login')

    expect(window.location.pathname).toBe('/')
    expect(window.location.hash).toBe('#/login')
  })

  it('preserves query params when setting hash routes', () => {
    window.history.replaceState(
      {},
      '',
      '/index.html?webDocumentId=electron-route-1',
    )

    setAppPath('/u/1')

    expect(window.location.pathname).toBe('/index.html')
    expect(window.location.search).toBe('?webDocumentId=electron-route-1')
    expect(window.location.hash).toBe('#/u/1')
  })

  it('sets app paths as hash routes from root', () => {
    setAppPath('/login')

    expect(window.location.pathname).toBe('/')
    expect(window.location.hash).toBe('#/login')
  })

  it('does not dispatch hashchange for an already canonical root hash route', async () => {
    window.location.hash = '#/login'
    await new Promise((resolve) => setTimeout(resolve, 0))

    let hashChanges = 0
    const listener = () => {
      hashChanges++
    }
    window.addEventListener('hashchange', listener)
    try {
      setAppPath('/login')
      await new Promise((resolve) => setTimeout(resolve, 0))
    } finally {
      window.removeEventListener('hashchange', listener)
    }

    expect(hashChanges).toBe(0)
  })
})
