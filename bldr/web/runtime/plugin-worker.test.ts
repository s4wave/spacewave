import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  PLUGIN_STARTUP_FAILURE_SHUTDOWN_DELAY_MS,
  parsePluginWorkerName,
  waitPluginStartupFailureShutdownDelay,
} from './plugin-worker.js'

describe('parsePluginWorkerName', () => {
  test('strips wrapper parameters from the worker identity', () => {
    expect(
      parsePluginWorkerName(
        'plugin/spacewave-app?s=/b/pd/app.mjs&t=quickjs&p=1',
      ),
    ).toBe('plugin/spacewave-app')
  })
})

describe('waitPluginStartupFailureShutdownDelay', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  test('waits before allowing failed plugin workers to shut down', async () => {
    vi.useFakeTimers()
    const complete = vi.fn()
    const wait = waitPluginStartupFailureShutdownDelay().then(complete)

    await vi.advanceTimersByTimeAsync(
      PLUGIN_STARTUP_FAILURE_SHUTDOWN_DELAY_MS - 1,
    )
    expect(complete).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    await wait
    expect(complete).toHaveBeenCalledTimes(1)
  })
})
