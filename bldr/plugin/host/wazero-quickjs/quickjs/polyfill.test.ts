import { describe, expect, it, vi } from 'vitest'

import {
  applyPolyfills,
  installRetainedSchedulerPolyfills,
  type QuickjsSchedulerTarget,
} from './polyfill.js'
import type { QuickjsGlobalScope } from './quickjs.js'

describe('installRetainedSchedulerPolyfills', () => {
  it('retains microtask callbacks until the host invokes them', () => {
    const scheduled: (() => void)[] = []
    const target = buildSchedulerTarget({
      queueMicrotask: (func) => {
        scheduled.push(func)
      },
    })
    const roots = installRetainedSchedulerPolyfills(target)
    const callback = vi.fn()

    target.queueMicrotask!(callback)

    expect(scheduled).toHaveLength(1)
    expect(scheduled[0]).not.toBe(callback)
    expect(roots.microtasks.size).toBe(1)

    scheduled[0]()

    expect(callback).toHaveBeenCalledTimes(1)
    expect(roots.microtasks.size).toBe(0)
  })

  it('releases one-shot timer callbacks after invocation or cancellation', () => {
    const scheduled = new Map<NodeJS.Timeout, () => void>()
    const target = buildSchedulerTarget({
      setTimeout: (func) => {
        const handle = scheduled.size + 1
        scheduled.set(handle as unknown as NodeJS.Timeout, func)
        return handle as unknown as NodeJS.Timeout
      },
      clearTimeout: (handle) => {
        scheduled.delete(handle)
      },
    })
    const roots = installRetainedSchedulerPolyfills(target)

    const callback = vi.fn()
    const first = target.setTimeout!(callback, 10)
    expect(roots.timeouts.size).toBe(1)
    scheduled.get(first)!()
    expect(callback).toHaveBeenCalledTimes(1)
    expect(roots.timeouts.size).toBe(0)

    const second = target.setTimeout!(callback, 10)
    expect(roots.timeouts.size).toBe(1)
    target.clearTimeout!(second)
    expect(roots.timeouts.size).toBe(0)
  })

  it('retains interval callbacks until cancellation', () => {
    const scheduled = new Map<NodeJS.Timeout, () => void>()
    const target = buildSchedulerTarget({
      setInterval: (func) => {
        const handle = scheduled.size + 1
        scheduled.set(handle as unknown as NodeJS.Timeout, func)
        return handle as unknown as NodeJS.Timeout
      },
      clearInterval: (handle) => {
        scheduled.delete(handle)
      },
    })
    const roots = installRetainedSchedulerPolyfills(target)
    const callback = vi.fn()

    const interval = target.setInterval!(callback, 10)
    expect(roots.intervals.size).toBe(1)

    scheduled.get(interval)!()

    expect(callback).toHaveBeenCalledTimes(1)
    expect(roots.intervals.size).toBe(1)

    target.clearInterval!(interval)

    expect(roots.intervals.size).toBe(0)
  })
})

describe('applyPolyfills', () => {
  it('exposes AbortSignal static helpers globally', () => {
    const target = buildPolyfillTarget()
    const polyfilled = applyPolyfills(target)

    expect(polyfilled.AbortSignal).toBe(polyfilled.AbortController.AbortSignal)
    expect(polyfilled.AbortSignal.abort).toBeTypeOf('function')
    expect(polyfilled.AbortSignal.timeout).toBeTypeOf('function')

    const signal = polyfilled.AbortSignal.abort('stopped')
    expect(signal.aborted).toBe(true)
    expect(signal.reason).toBe('stopped')
  })

  it('combines AbortSignals in input and event order', () => {
    const polyfilled = applyPolyfills(buildPolyfillTarget())
    const first = new polyfilled.AbortController()
    const second = new polyfilled.AbortController()
    first.abort('first')
    second.abort('second')

    const alreadyAborted = polyfilled.AbortSignal.any([
      first.signal,
      second.signal,
    ])
    expect(alreadyAborted.aborted).toBe(true)
    expect(alreadyAborted.reason).toBe('first')

    const laterFirst = new polyfilled.AbortController()
    const laterSecond = new polyfilled.AbortController()
    const combined = polyfilled.AbortSignal.any([
      laterFirst.signal,
      laterSecond.signal,
    ])
    laterSecond.abort('later-second')
    laterFirst.abort('later-first')
    expect(combined.aborted).toBe(true)
    expect(combined.reason).toBe('later-second')
  })

  it('removes AbortSignal.any listeners after the first abort', () => {
    const polyfilled = applyPolyfills(buildPolyfillTarget())
    const first = new polyfilled.AbortController()
    const second = new polyfilled.AbortController()
    const firstRemove = vi.spyOn(first.signal, 'removeEventListener')
    const secondRemove = vi.spyOn(second.signal, 'removeEventListener')
    const combined = polyfilled.AbortSignal.any([
      first.signal,
      second.signal,
      first.signal,
    ])

    first.abort('stopped')

    expect(combined.reason).toBe('stopped')
    expect(firstRemove).toHaveBeenCalledOnce()
    expect(secondRemove).toHaveBeenCalledOnce()
  })

  it('validates AbortSignal.any inputs and leaves an empty input pending', () => {
    const polyfilled = applyPolyfills(buildPolyfillTarget())
    const empty = polyfilled.AbortSignal.any([])
    expect(empty.aborted).toBe(false)
    expect(() =>
      polyfilled.AbortSignal.any([
        new polyfilled.AbortController().signal,
        new AbortController().signal,
      ]),
    ).toThrow(TypeError)
  })

  it('installs a ReadableStream that round-trips through a default reader', async () => {
    const target = buildPolyfillTarget()
    const polyfilled = applyPolyfills(target)

    expect(polyfilled.ReadableStream).toBeTypeOf('function')

    const data = new Uint8Array([1, 2, 3, 4])
    const stream = new polyfilled.ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(data)
        controller.close()
      },
    })
    const reader = stream.getReader()
    const first = await reader.read()
    expect(first.done).toBe(false)
    expect(Array.from(first.value!)).toEqual([1, 2, 3, 4])
    const second = await reader.read()
    expect(second.done).toBe(true)
    reader.releaseLock()
  })
})

function buildSchedulerTarget(
  overrides: Partial<QuickjsSchedulerTarget> & {
    setTimeout?: (func: () => void, delay: number) => NodeJS.Timeout
    clearTimeout?: (handle: NodeJS.Timeout) => void
    setInterval?: (func: () => void, delay: number) => NodeJS.Timeout
    clearInterval?: (handle: NodeJS.Timeout) => void
  },
): QuickjsSchedulerTarget {
  const setTimeout = overrides.setTimeout ?? vi.fn()
  const clearTimeout = overrides.clearTimeout ?? vi.fn()
  const setInterval = overrides.setInterval ?? vi.fn()
  const clearInterval = overrides.clearInterval ?? vi.fn()

  return {
    queueMicrotask: overrides.queueMicrotask,
    os: {
      setTimeout,
      clearTimeout,
      setInterval,
      clearInterval,
    },
  }
}

function buildPolyfillTarget(): QuickjsGlobalScope {
  return {
    console: {
      log: vi.fn(),
    },
    performance: {
      now: () => 0,
    },
    os: {
      setTimeout: ((func: () => void, delay: number) =>
        setTimeout(func, delay)) as QuickjsGlobalScope['os']['setTimeout'],
      clearTimeout: ((handle: NodeJS.Timeout) =>
        clearTimeout(handle)) as QuickjsGlobalScope['os']['clearTimeout'],
      setInterval: ((func: () => void, delay: number) =>
        setInterval(func, delay)) as QuickjsGlobalScope['os']['setInterval'],
      clearInterval: ((handle: NodeJS.Timeout) =>
        clearInterval(handle)) as QuickjsGlobalScope['os']['clearInterval'],
    },
  } as unknown as QuickjsGlobalScope
}
