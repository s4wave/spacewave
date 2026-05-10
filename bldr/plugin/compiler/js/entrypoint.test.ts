import { describe, expect, test, vi } from 'vitest'
import { retryWithAbort } from '@aptre/bldr'

import { entrypointRetryOpts, isEntrypointStreamReset } from './entrypoint.js'

describe('plugin JS entrypoint retry logging', () => {
  test('classifies stream resets as lifecycle retry noise', () => {
    const err = new Error('stream reset')
    err.name = 'StreamResetError'

    expect(isEntrypointStreamReset(err)).toBe(true)
    expect(isEntrypointStreamReset(new Error('stream reset'))).toBe(true)
    expect(isEntrypointStreamReset(new Error('different'))).toBe(false)
  })

  test('does not use generic retry warning for stream resets', async () => {
    vi.useFakeTimers()
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    let attempts = 0
    const controller = new AbortController()

    try {
      const result = retryWithAbort(
        controller.signal,
        async () => {
          attempts++
          if (attempts === 1) {
            const err = new Error('stream reset')
            err.name = 'StreamResetError'
            throw err
          }
        },
        entrypointRetryOpts('error configuring web view handlers'),
      )

      await vi.advanceTimersByTimeAsync(500)
      await result

      expect(attempts).toBe(2)
      expect(warn).not.toHaveBeenCalled()
      expect(error).not.toHaveBeenCalled()
    } finally {
      controller.abort()
      warn.mockRestore()
      error.mockRestore()
      vi.useRealTimers()
    }
  })

  test('logs unexpected retry errors with entrypoint context', async () => {
    vi.useFakeTimers()
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    let attempts = 0
    const controller = new AbortController()

    try {
      const result = retryWithAbort(
        controller.signal,
        async () => {
          attempts++
          if (attempts === 1) {
            throw new Error('boom')
          }
        },
        entrypointRetryOpts('error configuring web view handlers'),
      )

      await vi.advanceTimersByTimeAsync(500)
      await result

      expect(error).toHaveBeenCalledWith(
        'error configuring web view handlers: boom',
      )
      expect(error).toHaveBeenCalledWith(expect.any(Error))
    } finally {
      controller.abort()
      error.mockRestore()
      vi.useRealTimers()
    }
  })
})
