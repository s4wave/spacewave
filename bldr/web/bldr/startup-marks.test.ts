import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  markStartupBoundary,
  resetStartupMarksForTest,
  startupMarkEvent,
  startupMarkPrefix,
} from './startup-marks.js'

describe('startup marks', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    resetStartupMarksForTest()
  })

  it('emits ordered performance marks with stable names', () => {
    const mark = vi.spyOn(performance, 'mark').mockImplementation(() => {
      return {} as PerformanceMark
    })

    markStartupBoundary('runtime.wait-start', { source: 'browser' })
    markStartupBoundary('runtime.wait-ready', { source: 'browser' })

    expect(mark).toHaveBeenNthCalledWith(
      1,
      `${startupMarkPrefix}runtime.wait-start`,
      {
        detail: {
          label: 'runtime.wait-start',
          sequence: 1,
          source: 'browser',
        },
      },
    )
    expect(mark).toHaveBeenNthCalledWith(
      2,
      `${startupMarkPrefix}runtime.wait-ready`,
      {
        detail: {
          label: 'runtime.wait-ready',
          sequence: 2,
          source: 'browser',
        },
      },
    )
  })

  it('dispatches an event for live startup collectors', () => {
    const listener = vi.fn()
    globalThis.addEventListener(startupMarkEvent, listener)

    markStartupBoundary('worker.first-ready', {
      source: 'browser',
      workerId: 'worker-1',
    })

    globalThis.removeEventListener(startupMarkEvent, listener)
    expect(listener).toHaveBeenCalledTimes(1)
    expect(listener.mock.calls[0][0].detail).toEqual({
      name: `${startupMarkPrefix}worker.first-ready`,
      detail: {
        label: 'worker.first-ready',
        sequence: 1,
        source: 'browser',
        workerId: 'worker-1',
      },
    })
  })
})
