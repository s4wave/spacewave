import { describe, expect, it } from 'vitest'

import { ServiceWorkerFetchTracker } from './service-worker-fetch-tracker.js'

describe('ServiceWorkerFetchTracker', () => {
  it('tracks and releases fetches per client', () => {
    const tracker = new ServiceWorkerFetchTracker()

    const a = tracker.trackFetch('client-a')
    const b = tracker.trackFetch('client-a')
    const c = tracker.trackFetch('client-b')

    expect(tracker.getActiveFetchCount('client-a')).toBe(2)
    expect(tracker.getActiveFetchCount('client-b')).toBe(1)

    a.release()
    expect(tracker.getActiveFetchCount('client-a')).toBe(1)

    b.release()
    expect(tracker.getActiveFetchCount('client-a')).toBe(0)

    c.release()
    expect(tracker.getActiveFetchCount('client-b')).toBe(0)
  })

  it('aborts all active fetches for a client without touching others', () => {
    const tracker = new ServiceWorkerFetchTracker()

    const a = tracker.trackFetch('client-a')
    const b = tracker.trackFetch('client-a')
    const c = tracker.trackFetch('client-b')

    tracker.abortClient('client-a', new Error('client closed'))

    expect(a.abortController.signal.aborted).toBe(true)
    expect(b.abortController.signal.aborted).toBe(true)
    expect(c.abortController.signal.aborted).toBe(false)
    expect(tracker.getActiveFetchCount('client-a')).toBe(0)
    expect(tracker.getActiveFetchCount('client-b')).toBe(1)
  })
})
