import React, { act } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRoot, type Root } from 'react-dom/client'
import { ResourcesProvider } from './ResourcesContext.js'
import { useResource, type Resource } from './useResource.js'
import { useStreamingResource } from './useStreamingResource.js'
import { useMappedResource } from './useMappedResource.js'
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
    props.retryOnReleasedResource === undefined
      ? undefined
      : { retryOnReleasedResource: props.retryOnReleasedResource },
  )

  return React.createElement(
    'div',
    { 'data-handle-id': resource.value?.id ?? 0 },
    String(resource.value?.id ?? 0),
  )
}

function TestChildResource(props: {
  parent: Resource<FakeSDKHandle>
  factory: (value: FakeSDKHandle, signal: AbortSignal) => Promise<string>
}) {
  const resource = useResource(
    props.parent,
    async (value, signal) => props.factory(value, signal),
    [],
  )

  return React.createElement('div', {
    'data-loading': String(resource.loading),
    'data-value': resource.value ?? '',
  })
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
  const parent = useResource(
    async () => props.factory(props.version),
    [props.version],
  )
  const resource = useStreamingResource(
    parent,
    (value) =>
      props.streamFactory?.(value.version) ??
      streamValue(`stream-${value.version}`),
    [],
  )

  return React.createElement('div', {
    'data-loading': String(resource.loading),
    'data-value': resource.value ?? '',
    'data-error': resource.error?.message ?? '',
  })
}

function TestStreamingFromParent(props: {
  parent: Resource<{ version: number }>
  streamFactory?: (version: number) => AsyncIterable<string>
}) {
  const resource = useStreamingResource(
    props.parent,
    (value) =>
      props.streamFactory?.(value.version) ??
      streamValue(`stream-${value.version}`),
    [props.streamFactory],
  )

  return React.createElement('div', {
    'data-loading': String(resource.loading),
    'data-value': resource.value ?? '',
    'data-error': resource.error?.message ?? '',
  })
}

function TestMappedValue(props: {
  source: Resource<string>
  mapValue: (value: string) => string
}) {
  const resource = useMappedResource(
    props.source,
    (value) => props.mapValue(value),
    [props.mapValue],
  )

  return React.createElement(
    'div',
    {
      'data-loading': String(resource.loading),
      'data-value': resource.value ?? '',
      'data-error': resource.error?.message ?? '',
    },
    React.createElement(
      'button',
      { type: 'button', onClick: resource.retry },
      'retry',
    ),
  )
}

type ManualAsyncIterableController<T> = {
  iterable: AsyncIterable<T>
  push(value: T): void
  fail(err: unknown): void
  finish(): void
}

function createManualAsyncIterable<T>(): ManualAsyncIterableController<T> {
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

type Deferred<T> = {
  promise: Promise<T>
  resolve(value: T): void
}

function deferred<T>(): Deferred<T> {
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
    const props = {
      client: client as never,
      children: React.createElement(TestHandle, { factory }),
    }

    await act(async () => {
      root?.render(React.createElement(ResourcesProvider, props))
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
    const props = {
      client: client as never,
      children: React.createElement(TestHandle, {
        factory,
        retryOnReleasedResource: false,
      }),
    }

    await act(async () => {
      root?.render(React.createElement(ResourcesProvider, props))
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

  it('aborts child work when its SDK parent is released', async () => {
    const client = new FakeResourceClient()
    const parentValue = buildHandle(1)
    const parent: Resource<FakeSDKHandle> = {
      value: parentValue,
      loading: false,
      error: null,
      retry: vi.fn(),
    }
    let childSignal: AbortSignal | undefined
    const childFactory = vi.fn(
      async (_value: FakeSDKHandle, signal: AbortSignal): Promise<string> => {
        childSignal = signal
        await new Promise<string>(() => {})
        return 'unreachable'
      },
    )
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(ResourcesProvider, {
          client: client as never,
          children: React.createElement(TestChildResource, {
            parent,
            factory: childFactory,
          }),
        }),
      )
      await flush()
    })

    expect(childFactory).toHaveBeenCalledOnce()
    expect(childSignal?.aborted).toBe(false)

    await act(async () => {
      client.emit({ resourceId: 1, reason: 'client-released' })
      expect(childSignal?.aborted).toBe(true)
    })
  })

  it('keeps the previous resource value visible while a dependency reload is pending', async () => {
    const pending = new Map<number, Deferred<string>>()
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

  it('suppresses stale async completions after a dependency reload', async () => {
    const pending = new Map<number, Deferred<string>>()
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

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe('')

    await act(async () => {
      root?.render(React.createElement(TestValue, { factory, version: 2 }))
      await flush()
    })

    await act(async () => {
      pending.get(2)?.resolve('value-2')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'value-2',
    )

    await act(async () => {
      pending.get(1)?.resolve('stale-value-1')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'value-2',
    )
    expect(factory).toHaveBeenCalledTimes(2)
  })

  it('keeps the previous streamed value visible while the parent reloads', async () => {
    const pending = new Map<number, Deferred<{ version: number }>>()
    const factory = vi.fn((version: number) => {
      const next = deferred<{ version: number }>()
      pending.set(version, next)
      return next.promise
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(TestStreamValue, { factory, version: 1 }),
      )
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
      root?.render(
        React.createElement(TestStreamValue, { factory, version: 2 }),
      )
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

  it('marks the stream loading while a parent replacement is still pending', async () => {
    const streams = new Map<number, ManualAsyncIterableController<string>>()
    const streamFactory = vi.fn((version: number) => {
      const next = createManualAsyncIterable<string>()
      streams.set(version, next)
      return next.iterable
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    const renderParent = (parent: Resource<{ version: number }>) => {
      root?.render(
        React.createElement(TestStreamingFromParent, {
          parent,
          streamFactory,
        }),
      )
    }

    await act(async () => {
      renderParent({
        value: { version: 1 },
        loading: false,
        error: null,
        retry: () => {},
      })
      await flush()
    })

    await act(async () => {
      streams.get(1)?.push('stream-1')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1',
    )

    await act(async () => {
      renderParent({
        value: { version: 2 },
        loading: true,
        error: null,
        retry: () => {},
      })
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1',
    )
    expect(streamFactory).toHaveBeenCalledTimes(1)

    await act(async () => {
      renderParent({
        value: { version: 2 },
        loading: false,
        error: null,
        retry: () => {},
      })
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )
    expect(streamFactory).toHaveBeenCalledTimes(2)

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
  })

  it('ignores stale stream errors while a parent replacement is in flight', async () => {
    const pending = new Map<number, Deferred<{ version: number }>>()
    const streams = new Map<number, ManualAsyncIterableController<string>>()
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

  it('retries a current stream abort without staying loading forever', async () => {
    const streams: Array<ManualAsyncIterableController<string>> = []
    const factory = vi.fn(async (version: number) => ({ version }))
    const streamFactory = vi.fn(() => {
      const next = createManualAsyncIterable<string>()
      streams.push(next)
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
      await flush()
    })

    expect(streamFactory).toHaveBeenCalledTimes(1)

    await act(async () => {
      streams[0].fail(new Error('ERR_RPC_ABORT'))
      await flush()
      await flush()
      await flush()
    })

    expect(streamFactory).toHaveBeenCalledTimes(2)
    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')

    await act(async () => {
      streams[1].push('stream-1-retry')
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'stream-1-retry',
    )
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')
  })

  it('surfaces repeated current stream aborts instead of hanging', async () => {
    const streams: Array<ManualAsyncIterableController<string>> = []
    const factory = vi.fn(async (version: number) => ({ version }))
    const streamFactory = vi.fn(() => {
      const next = createManualAsyncIterable<string>()
      streams.push(next)
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
      await flush()
    })

    await act(async () => {
      streams[0].fail(new Error('ERR_RPC_ABORT'))
      await flush()
      await flush()
      await flush()
    })

    expect(streamFactory).toHaveBeenCalledTimes(2)

    await act(async () => {
      streams[1].fail(new Error('ERR_RPC_ABORT'))
      await flush()
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe('')
    expect(container.firstElementChild?.getAttribute('data-error')).toBe(
      'ERR_RPC_ABORT',
    )
  })

  it('settles as not loading when the parent resolves to null', async () => {
    const factory = vi.fn(async (): Promise<{ version: number } | null> => null)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(TestStreamValue, {
          factory: factory as never,
          version: 1,
        }),
      )
      await flush()
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe('')
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')
  })

  it('settles as not loading when an explicit parent has no terminal value', async () => {
    const streamFactory = vi.fn((version: number) =>
      streamValue(`stream-${version}`),
    )
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(TestStreamingFromParent, {
          parent: {
            value: null,
            loading: false,
            error: null,
            retry: () => {},
          },
          streamFactory,
        }),
      )
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe('')
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')
    expect(streamFactory).not.toHaveBeenCalled()
  })

  it('propagates mapped resource loading, error, and retry while mapping non-null values', async () => {
    let retryCalls = 0
    const error = new Error('source failed')
    const mapValue = vi.fn((value: string) => `mapped:${value}`)
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        React.createElement(TestMappedValue, {
          source: {
            value: 'source-value',
            loading: true,
            error,
            retry: () => {
              retryCalls += 1
            },
          },
          mapValue,
        }),
      )
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'true',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe(
      'mapped:source-value',
    )
    expect(container.firstElementChild?.getAttribute('data-error')).toBe(
      'source failed',
    )
    expect(mapValue).toHaveBeenCalledTimes(1)
    expect(mapValue).toHaveBeenCalledWith('source-value')

    await act(async () => {
      container
        ?.querySelector('button')
        ?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await flush()
    })

    expect(retryCalls).toBe(1)

    mapValue.mockClear()

    await act(async () => {
      root?.render(
        React.createElement(TestMappedValue, {
          source: {
            value: null,
            loading: false,
            error: null,
            retry: () => {
              retryCalls += 1
            },
          },
          mapValue,
        }),
      )
      await flush()
    })

    expect(container.firstElementChild?.getAttribute('data-loading')).toBe(
      'false',
    )
    expect(container.firstElementChild?.getAttribute('data-value')).toBe('')
    expect(container.firstElementChild?.getAttribute('data-error')).toBe('')
    expect(mapValue).not.toHaveBeenCalled()
  })
})
