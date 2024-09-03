import { describe, test, expect, vi } from 'vitest'
import { Retry, constantBackoff } from './retry.js'

describe('Retry', () => {
  test('should resolve immediately on success', async () => {
    const fn = vi.fn().mockResolvedValue('success')
    const retry = new Retry(fn)

    const result = await retry.result
    expect(result).toBe('success')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  test('should retry on failure and then succeed', async () => {
    const fn = vi.fn()
      .mockRejectedValueOnce(new Error('fail'))
      .mockResolvedValueOnce('success')

    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })

    const result = await retry.result
    expect(result).toBe('success')
    expect(fn).toHaveBeenCalledTimes(2)
  })

  test('should call errorCb on failure', async () => {
    const error = new Error('fail')
    const fn = vi.fn().mockRejectedValue(error)
    const errorCb = vi.fn()

    const retry = new Retry(fn, { backoffFn: constantBackoff(10), errorCb })

    // Wait for a short time to allow the first retry attempt
    await new Promise(resolve => setTimeout(resolve, 20))

    expect(errorCb).toHaveBeenCalledWith(error)
    retry.cancel() // Cancel to stop further retries
  })

  test('should cancel retry', async () => {
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })

    setTimeout(() => retry.cancel(), 5)

    await expect(retry.result).rejects.toThrow('fail')
    expect(fn).toHaveBeenCalledTimes(1)
    expect(retry.canceled).toBe(true)
  })

  test('should retry multiple times before succeeding', async () => {
    const fn = vi.fn()
      .mockRejectedValueOnce(new Error('fail1'))
      .mockRejectedValueOnce(new Error('fail2'))
      .mockRejectedValueOnce(new Error('fail3'))
      .mockResolvedValueOnce('success')

    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })

    const result = await retry.result
    expect(result).toBe('success')
    expect(fn).toHaveBeenCalledTimes(4)
  })

  test('should fail after maximum retries', async () => {
    const maxRetries = 5
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const retry = new Retry(fn, { backoffFn: constantBackoff(10) })

    setTimeout(() => retry.cancel(), (maxRetries + 1) * 10)

    await expect(retry.result).rejects.toThrow('fail')
    expect(fn).toHaveBeenCalledTimes(maxRetries + 1)
  })

  test('should stop retrying when abortSignal is triggered', async () => {
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const abortController = new AbortController()
    const retry = new Retry(fn, { backoffFn: constantBackoff(10), abortSignal: abortController.signal })

    setTimeout(() => abortController.abort(), 25)

    await expect(retry.result).rejects.toThrow('fail')
    expect(fn).toHaveBeenCalledTimes(3)
    expect(retry.canceled).toBe(true)
  })

  test('should return immediately if abort controller is canceled before execution', async () => {
    const fn = vi.fn().mockRejectedValue(new Error('fail'))
    const abortController = new AbortController()
    abortController.abort() // Cancel before creating Retry instance

    const retry = new Retry(fn, { backoffFn: constantBackoff(10), abortSignal: abortController.signal })

    await expect(retry.result).rejects.toThrow('fail')
    expect(fn).toHaveBeenCalledTimes(0)
    expect(retry.canceled).toBe(true)
  })
})
