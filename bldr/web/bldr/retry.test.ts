import { describe, test, expect, vi } from 'vitest'
import { Retry, constantBackoff, exponentialBackoff } from './retry.js'

describe('Retry', () => {
  test('should resolve immediately on success', async () => {
    // Run a function that succeeds at once.
    const fn = vi.fn().mockResolvedValue('success')
    const retry = new Retry(fn)

    // It resolves after one call.
    const result = await retry.result
    expect(result).toBe('success')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  test('removes abort listener after success', async () => {
    // Watch the abort signal's listeners while a call succeeds.
    const controller = new AbortController()
    const removeEventListener = vi.spyOn(
      controller.signal,
      'removeEventListener',
    )
    const fn = vi.fn().mockResolvedValue('success')
    const retry = new Retry(fn, { abortSignal: controller.signal })
    await expect(retry.result).resolves.toBe('success')

    // Success removes the abort listener.
    expect(removeEventListener).toHaveBeenCalledWith(
      'abort',
      expect.any(Function),
    )
  })

  test('should retry on failure and then succeed', async () => {
    // Fail once, then succeed.
    const fn = vi
      .fn()
      .mockRejectedValueOnce(new Error('fail'))
      .mockResolvedValueOnce('success')
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })

    // The second call's result resolves the retry.
    const result = await retry.result
    expect(result).toBe('success')
    expect(fn).toHaveBeenCalledTimes(2)
  })

  test('should call errorCb on failure', async () => {
    // Fail every call and wait for the first retry.
    const error = new Error('fail')
    const fn = vi.fn().mockRejectedValue(error)
    const errorCb = vi.fn()
    const retry = new Retry(fn, { backoffFn: constantBackoff(10), errorCb })
    await new Promise((resolve) => setTimeout(resolve, 20))

    // The callback sees the failure.
    expect(errorCb).toHaveBeenCalledWith(error)
    retry.cancel()
  })

  test('should warn by default on failure', async () => {
    // Fail every call without an error callback.
    const error = new Error('fail')
    const fn = vi.fn().mockRejectedValue(error)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })
    await new Promise((resolve) => setTimeout(resolve, 20))

    // The default callback warns.
    expect(warn).toHaveBeenCalledWith('Retry: retrying after error', {
      error,
    })
    retry.cancel()
    warn.mockRestore()
  })

  test('should cancel retry', async () => {
    // Cancel during the first backoff.
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })
    setTimeout(() => retry.cancel(), 5)

    // The retry rejects with the last error and makes no further calls.
    await expect(retry.result).rejects.toThrow('fail')
    expect(fn).toHaveBeenCalledTimes(1)
    expect(retry.canceled).toBe(true)
  })

  test('removes abort listener on cancel', async () => {
    // Watch the abort signal's listeners while failing calls retry.
    const controller = new AbortController()
    const removeEventListener = vi.spyOn(
      controller.signal,
      'removeEventListener',
    )
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const retry = new Retry(fn, {
      abortSignal: controller.signal,
      backoffFn: constantBackoff(10),
    })

    // Cancel after a few attempts.
    await new Promise((resolve) => setTimeout(resolve, 20))
    retry.cancel()
    await expect(retry.result).rejects.toThrow('fail')

    // Cancel removes the abort listener.
    expect(removeEventListener).toHaveBeenCalledWith(
      'abort',
      expect.any(Function),
    )
  })

  test('should retry multiple times before succeeding', async () => {
    // Fail three times, then succeed.
    const fn = vi
      .fn()
      .mockRejectedValueOnce(new Error('fail1'))
      .mockRejectedValueOnce(new Error('fail2'))
      .mockRejectedValueOnce(new Error('fail3'))
      .mockResolvedValueOnce('success')
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })

    // The fourth call resolves the retry.
    const result = await retry.result
    expect(result).toBe('success')
    expect(fn).toHaveBeenCalledTimes(4)
  })

  test('should fail after repeated retries when canceled', async () => {
    // Fail every call and cancel after about five backoffs.
    const callsBeforeCancel = 5
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })
    setTimeout(() => retry.cancel(), callsBeforeCancel * 10)

    // The retry rejects after a bounded number of calls.
    await expect(retry.result).rejects.toThrow('fail')
    expect(fn.mock.calls.length).toBeGreaterThan(0)
    expect(fn.mock.calls.length).toBeLessThanOrEqual(callsBeforeCancel)
    warn.mockRestore()
  })

  test('should stop retrying when abortSignal is triggered', async () => {
    // Fail every call and abort during the backoffs.
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const abortController = new AbortController()
    const retry = new Retry(fn, {
      backoffFn: constantBackoff(10),
      abortSignal: abortController.signal,
    })
    setTimeout(() => abortController.abort(), 25)

    // The abort cancels the retry.
    await expect(retry.result).rejects.toThrow('fail')
    expect(fn.mock.calls.length).toBeGreaterThan(0)
    expect(fn.mock.calls.length).toBeLessThanOrEqual(3)
    expect(retry.canceled).toBe(true)
  })

  test('should return immediately if abort controller is canceled before execution', async () => {
    // Abort before the retry starts.
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const abortController = new AbortController()
    abortController.abort()
    const retry = new Retry(fn, {
      backoffFn: constantBackoff(10),
      abortSignal: abortController.signal,
    })

    // The retry rejects without calling the function.
    await expect(retry.result).rejects.toThrow('fail')
    expect(fn).toHaveBeenCalledTimes(0)
    expect(retry.canceled).toBe(true)
  })

  test('exponentialBackoff doubles a jittered wait and resets', () => {
    // Each wait lies between half and all of the doubling bound.
    const backoff = exponentialBackoff(100, 400)
    const bounds = [100, 200, 400, 400]
    for (const bound of bounds) {
      const wait = backoff()
      expect(wait).toBeGreaterThanOrEqual(bound / 2)
      expect(wait).toBeLessThanOrEqual(bound)
      expect(backoff.capped()).toBe(bound === 400)
    }

    // A reset returns to the initial bound.
    backoff.reset()
    expect(backoff.capped()).toBe(false)
    expect(backoff()).toBeLessThanOrEqual(100)
  })
})
