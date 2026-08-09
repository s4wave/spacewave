import { afterEach, describe, expect, it } from 'vitest'

import {
  getAppNavigationGeneration,
  getAppPath,
  isPathnameAppRoute,
  normalizeAppPath,
  setAppPath,
  subscribeAppPath,
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
  it('advances the navigation generation across a history round trip', async () => {
    setAppPath('/start')
    await new Promise((resolve) => setTimeout(resolve, 0))
    const start = getAppNavigationGeneration()

    setAppPath('/files')
    setAppPath('/start')
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(getAppPath()).toBe('/start')
    expect(getAppNavigationGeneration()).toBeGreaterThan(start)
  })

  it('holds the navigation generation when the path does not change', async () => {
    setAppPath('/start')
    await new Promise((resolve) => setTimeout(resolve, 0))
    const start = getAppNavigationGeneration()

    setAppPath('/start')
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(getAppNavigationGeneration()).toBe(start)
  })

  it('advances the navigation generation on a raw hash write, before hashchange runs', async () => {
    setAppPath('/start')
    await new Promise((resolve) => setTimeout(resolve, 0))
    const start = getAppNavigationGeneration()

    // A caller writing the hash directly, as app/debug/spacewave-global.ts
    // does, moves the document now and queues hashchange for later. Read the
    // generation in the same task, before that event can run.
    window.location.hash = '#/files'

    expect(getAppNavigationGeneration()).toBeGreaterThan(start)
  })

  it('counts a raw hash write once when hashchange later runs', async () => {
    setAppPath('/start')
    await new Promise((resolve) => setTimeout(resolve, 0))
    const start = getAppNavigationGeneration()

    window.location.hash = '#/files'
    const afterWrite = getAppNavigationGeneration()
    await new Promise((resolve) => setTimeout(resolve, 0))

    expect(afterWrite).toBe(start + 1)
    expect(getAppNavigationGeneration()).toBe(afterWrite)
  })
  it('completes a two-subscriber fanout before callback navigation', () => {
    const notifications: string[] = []
    const unsubscribeFirst = subscribeAppPath(() => {
      notifications.push('first')
      if (notifications.length === 1) setAppPath('/second')
    })
    const unsubscribeSecond = subscribeAppPath(() => {
      notifications.push('second')
    })

    setAppPath('/first')

    expect(getAppPath()).toBe('/second')
    expect(notifications).toEqual(['first', 'second', 'first', 'second'])
    unsubscribeFirst()
    unsubscribeSecond()
  })

  it('coalesces multiple nested paths into one follow-up fanout', () => {
    const notifications: string[] = []
    const unsubscribeFirst = subscribeAppPath(() => {
      notifications.push('first')
      if (notifications.length !== 1) return
      setAppPath('/second')
      setAppPath('/third')
      setAppPath('/fourth')
    })
    const unsubscribeSecond = subscribeAppPath(() => {
      notifications.push('second')
    })

    setAppPath('/first')

    expect(getAppPath()).toBe('/fourth')
    expect(notifications).toEqual(['first', 'second', 'first', 'second'])
    unsubscribeFirst()
    unsubscribeSecond()
  })

  it('applies subscription changes after the current fanout', () => {
    const notifications: string[] = []
    let unsubscribeThird = () => {}
    let changedSubscriptions = false
    const unsubscribeFirst = subscribeAppPath(() => {
      notifications.push('first')
      if (changedSubscriptions) return
      changedSubscriptions = true
      unsubscribeSecond()
      unsubscribeThird = subscribeAppPath(() => notifications.push('third'))
    })
    const unsubscribeSecond = subscribeAppPath(() => {
      notifications.push('second')
    })

    setAppPath('/first')
    expect(notifications).toEqual(['first', 'second'])

    setAppPath('/second')
    expect(notifications).toEqual(['first', 'second', 'first', 'third'])
    unsubscribeFirst()
    unsubscribeThird()
  })

  it('ignores duplicate native events for an observed path', () => {
    let notifications = 0
    const unsubscribe = subscribeAppPath(() => notifications++)

    setAppPath('/first')
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    window.dispatchEvent(new PopStateEvent('popstate'))

    expect(notifications).toBe(1)
    unsubscribe()
  })

  it('resets its observed path after the final subscriber leaves', () => {
    const paths: string[] = []
    const unsubscribeFirst = subscribeAppPath(() => paths.push(getAppPath()))
    setAppPath('/first')
    unsubscribeFirst()

    setAppPath('/second')
    const unsubscribeSecond = subscribeAppPath(() => paths.push(getAppPath()))
    setAppPath('/third')

    expect(paths).toEqual(['/first', '/third'])
    unsubscribeSecond()
  })

  it('completes fanout and remains usable when a subscriber throws', () => {
    const failure = new Error('subscriber failed')
    let shouldThrow = true
    const notifications: string[] = []
    const unsubscribeFirst = subscribeAppPath(() => {
      notifications.push('first')
      if (shouldThrow) {
        shouldThrow = false
        throw failure
      }
    })
    const unsubscribeSecond = subscribeAppPath(() => {
      notifications.push('second')
    })

    expect(() => setAppPath('/first')).toThrow(failure)
    expect(notifications).toEqual(['first', 'second'])

    setAppPath('/second')
    expect(notifications).toEqual(['first', 'second', 'first', 'second'])
    unsubscribeFirst()
    unsubscribeSecond()
  })

  it('notifies each subscriber once per logical path transition', async () => {
    window.history.replaceState({}, '', '/login')
    const paths: string[] = []
    const unsubscribe = subscribeAppPath(() => paths.push(getAppPath()))

    setAppPath('/g/layout')
    expect(paths).toEqual(['/g/layout'])

    setAppPath('/files')
    expect(paths).toEqual(['/g/layout', '/files'])
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(paths).toEqual(['/g/layout', '/files'])

    window.location.hash = '#/docs'
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(paths).toEqual(['/g/layout', '/files', '/docs'])

    window.history.back()
    expect(paths).toEqual(['/g/layout', '/files', '/docs', '/files'])
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(paths).toEqual(['/g/layout', '/files', '/docs', '/files'])

    window.history.replaceState({}, '', '/index.html?document=test')
    setAppPath('/display')
    expect(paths).toEqual([
      '/g/layout',
      '/files',
      '/docs',
      '/files',
      '/display',
    ])

    unsubscribe()
    window.location.hash = '#/ignored'
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(paths).toEqual([
      '/g/layout',
      '/files',
      '/docs',
      '/files',
      '/display',
    ])
  })
})
