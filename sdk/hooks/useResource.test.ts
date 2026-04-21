import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act } from 'react-dom/test-utils'
import { createRoot, type Root } from 'react-dom/client'
import { ResourcesProvider } from './ResourcesContext.js'
import { useResource } from './useResource.js'
import { useStreamingResource } from './useStreamingResource.js'
import { Resource as SDKResource } from '../resource/resource.js'
import type {
  ClientResourceRef,
  ResourceReleasedEvent,
} from '../resource/client.js'

class FakeResourceClient {
  private listeners = new Set<(event: ResourceReleasedEvent) => void>()

  onResourceReleased(
    callback: (event: ResourceReleasedEvent) => void,
  ): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  emit(event: ResourceReleasedEvent): void {
    this.listeners.forEach((listener) => listener(event))
  }
}

class FakeSDKHandle extends SDKResource {}

function buildHandle(id: number): FakeSDKHandle {
  const ref = {} as ClientResourceRef
  Object.assign(ref, {
    resourceId: id,
    released: false,
    client: {} as never,
    createRef: () => ref,
    createResource: () => {
      throw new Error('not implemented')
    },
    release: () => {},
    [Symbol.dispose]: () => {},
  })
  return new FakeSDKHandle(ref)
}

function TestHandle(props: {
  factory: () => Promise<FakeSDKHandle>
  retryOnReleasedResource?: boolean
}) {
  const resource = useResource(
    async (_signal, cleanup) => cleanup(await props.factory()),
    [],
    props.retryOnReleasedResource === undefined ?
      undefined
    : { retryOnReleasedResource: props.retryOnReleasedResource },
  )

  return React.createElement(
    'div',
    { 'data-handle-id': resource.value?.id ?? 0 },
    String(resource.value?.id ?? 0),
  )
}

function TestValue(props: {
  factory: (version: number) => Promise<string>
  version: number
}) {
  const resource = useResource(
    async () => props.factory(props.version),
    [props.version],
  )

  return React.createElement('div', {
    'data-loading': String(resource.loading),
    'data-value': resource.value ?? '',
    'data-error': resource.error?.message ?? '',
  })
}

async function* streamValue(value: string): AsyncIterable<string> {
  yield value
}

function TestStreamValue(props: {
  factory: (version: number) => Promise<{ version: number }>
  version: number
  streamFactory?: (version: number) => AsyncIterable<string>
}) {
  const parent = useResource(async () => props.factory(props.version), [
    props.version,
  ])
  const resource = useStreamingResource(
    parent,
    (value) =>
      props.streamFactory?.(value.version) ?? streamValue(`stream-${value.version}`),
    [],
  )

  return React.createElement('div', {
    'data-loading': String(resource.loading),
    'data-value': resource.value ?? '',
    'data-error': resource.error?.message ?? '',
  })
}

function createManualAsyncIterable<T>() {
  const queue: Array<IteratorResult<T>> = []
  const waiters: Array<{
    resolve: (value: IteratorResult<T>) => void
    reject: (err: unknown) => void
  }> = []
  let failure: unknown = null
  let done = false

  return {
    iterable: {
      [Symbol.asyncIterator]() {
        return {
          next(): Promise<IteratorResult<T>> {
            if (failure) {
              return Promise.reject(failure)
            }
            if (queue.length > 0) {
              return Promise.resolve(queue.shift()!)
            }
            if (done) {
              return Promise.resolve({
                done: true,
                value: undefined,
              } as IteratorResult<T>)
            }
            return new Promise<IteratorResult<T>>((resolve, reject) => {
              waiters.push({ resolve, reject })
            })
          },
        }
      },
    } satisfies AsyncIterable<T>,
    push(value: T) {
      const waiter = waiters.shift()
      if (waiter) {
        waiter.resolve({ done: false, value })
        return
      }
      queue.push({ done: false, value })
    },
    fail(err: unknown) {
      failure = err
      const currentWaiters = waiters.splice(0, waiters.length)
      currentWaiters.forEach((waiter) => waiter.reject(err))
    },
    finish() {
      done = true
      const currentWaiters = waiters.splice(0, waiters.length)
      currentWaiters.forEach((waiter) =>
        waiter.resolve({
          done: true,
          value: undefined,
        } as IteratorResult<T>),
      )
    },
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

async function flush(): Promise<void> {
  await Promise.resolve()
}

describe('useResource', () => {
  let container: HTMLDivElement | null = null
  let root: Root | null = null

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount()
        await flush()
      })
    }
    root = null
    container?.remove()
    container = null
  })

  it('retries released SDK resources by default', async () => {
    const client = new FakeResourceClient()
    let nextId = 1
    const factory = vi.fn(async () => buildHandle(nextId++))
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(
          ResourcesProvider,
          { client: client as never },
          React.createElement(TestHandle, { factory }),
        ),
      )
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-handle-id')).toBe(
      '1',
    )

    await act(async () => {
      client.emit({ resourceId: 1, reason: 'server-released' })
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-handle-id')).toBe(
      '2',
    )
    expect(factory).toHaveBeenCalledTimes(2)
  })

  it('allows opting out of release-triggered retries', async () => {
    const client = new FakeResourceClient()
    let nextId = 1
    const factory = vi.fn(async () => buildHandle(nextId++))
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(
          ResourcesProvider,
          { client: client as never },
          React.createElement(TestHandle, {
            factory,
            retryOnReleasedResource: false,
          }),
        ),
      )
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-handle-id')).toBe(
      '1',
    )

    await act(async () => {
      client.emit({ resourceId: 1, reason: 'server-released' })
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-handle-id')).toBe(
      '1',
    )
    expect(factory).toHaveBeenCalledTimes(1)
  })

  it('keeps the previous resource value visible while a dependency reload is pending', async () => {
    const pending = new Map<number, ReturnType<typeof deferred<string>>>()
    const factory = vi.fn((version: number) => {
      const next = deferred<string>()
      pending.set(version, next)
      return next.promise
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(React.createElement(TestValue, { factory, version: 1 }))
      await flush()
    })

    await act(async () => {
      pending.get(1)?.resolve('value-1')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'value-1',
    )
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )

    await act(async () => {
      root?.render(React.createElement(TestValue, { factory, version: 2 }))
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'value-1',
    )
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )

    await act(async () => {
      pending.get(2)?.resolve('value-2')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'value-2',
    )
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
  })

  it('keeps the previous streamed value visible while the parent reloads', async () => {
    const pending = new Map<number, ReturnType<typeof deferred<{ version: number }>>>()
    const factory = vi.fn((version: number) => {
      const next = deferred<{ version: number }>()
      pending.set(version, next)
      return next.promise
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(React.createElement(TestStreamValue, { factory, version: 1 }))
      await flush()
    })

    await act(async () => {
      pending.get(1)?.resolve({ version: 1 })
      await flush()
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1',
    )
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )

    await act(async () => {
      root?.render(React.createElement(TestStreamValue, { factory, version: 2 }))
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1',
    )
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )

    await act(async () => {
      pending.get(2)?.resolve({ version: 2 })
      await flush()
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-2',
    )
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
  })

  it('ignores stale stream errors while a parent replacement is in flight', async () => {
    const pending = new Map<number, ReturnType<typeof deferred<{ version: number }>>>()
    const streams = new Map<number, ReturnType<typeof createManualAsyncIterable<string>>>()
    const factory = vi.fn((version: number) => {
      const next = deferred<{ version: number }>()
      pending.set(version, next)
      return next.promise
    })
    const streamFactory = vi.fn((version: number) => {
      const next = createManualAsyncIterable<string>()
      streams.set(version, next)
      return next.iterable
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(TestStreamValue, {
          factory,
          version: 1,
          streamFactory,
        }),
      )
      await flush()
    })

    await act(async () => {
      pending.get(1)?.resolve({ version: 1 })
      await flush()
      await flush()
    })

    await act(async () => {
      streams.get(1)?.push('stream-1')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1',
    )
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')

    await act(async () => {
      root?.render(
        React.createElement(TestStreamValue, {
          factory,
          version: 2,
          streamFactory,
        }),
      )
      await flush()
    })

    await act(async () => {
      streams.get(1)?.fail(new Error('released handle'))
      pending.get(2)?.resolve({ version: 2 })
      await flush()
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1',
    )
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')

    await act(async () => {
      streams.get(2)?.push('stream-2')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-2',
    )
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')
  })

  it('settles as not loading when the parent resolves to null', async () => {
    const factory = vi.fn(async () => null)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(React.createElement(TestStreamValue, { factory, version: 1 }))
      await flush()
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe('')
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')
  })
})
