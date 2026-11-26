import React, { act, useEffect } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRoot, type Root } from 'react-dom/client'
import { ResourcesProvider } from './ResourcesContext.js'
import { useResource, type Resource } from './useResource.js'
import {
  useStreamingResource,
  type UseStreamingResourceOptions,
} from './useStreamingResource.js'
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

function TestRetryChild(props: {
  parent: Resource<string>
  factory: (value: string) => Promise<string>
}) {
  const resource = useResource(
    props.parent,
    async (value) => props.factory(value),
    [],
  )

  return React.createElement(
    'button',
    { type: 'button', onClick: resource.retry },
    'retry',
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

/** Row is a streamed value with an identity separate from its content. */
type Row = { id: number; label: string }

/** TestStreamOptions renders a stream of rows and records each distinct value. */
function TestStreamOptions(props: {
  streamFactory: () => AsyncIterable<Row>
  options: UseStreamingResourceOptions<Row>
  seen: Row[]
}) {
  // Stream rows from a trivial parent.
  const parent = useResource(async () => ({}), [])
  const resource = useStreamingResource(
    parent,
    () => props.streamFactory(),
    [],
    props.options,
  )

  // Record each value the consumer receives.
  const { seen } = props
  useEffect(() => {
    if (resource.value && seen.at(-1) !== resource.value) {
      seen.push(resource.value)
    }
  }, [resource.value, seen])

  return React.createElement('div', {
    'data-loading': String(resource.loading),
    'data-value': resource.value?.label ?? '',
    'data-error': resource.error?.message ?? '',
  })
}

/** ManualAsyncIterableController pushes, fails or ends a manual stream. */
type ManualAsyncIterableController<T> = {
  iterable: AsyncIterable<T>
  push(value: T): void
  fail(err: unknown): void
  finish(): void
}

/** createManualAsyncIterable returns a stream the test drives by hand. */
function createManualAsyncIterable<T>(): ManualAsyncIterableController<T> {
  // Queue pushed results until a reader waits for them.
  const queue: Array<IteratorResult<T>> = []
  const waiters: Array<{
    resolve: (value: IteratorResult<T>) => void
    reject: (err: unknown) => void
  }> = []
  let failure: unknown = null
  let done = false

  // Hand each result to the oldest waiter, or queue it.
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

/** Deferred is a promise the test resolves by hand. */
type Deferred<T> = {
  promise: Promise<T>
  resolve(value: T): void
}

/** deferred returns an unresolved Deferred. */
function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

/** flush lets one round of pending promise callbacks run. */
async function flush(): Promise<void> {
  await Promise.resolve()
}

/** deferredFactory returns a factory whose promises the test resolves by version. */
function deferredFactory<T>() {
  const pending = new Map<number, Deferred<T>>()
  const factory = vi.fn((version: number) => {
    const next = deferred<T>()
    pending.set(version, next)
    return next.promise
  })
  return { pending, factory }
}

/** manualStreams returns a stream factory that records each manual stream. */
function manualStreams<T>() {
  const streams: Array<ManualAsyncIterableController<T>> = []
  const streamFactory = vi.fn((_version?: number) => {
    const next = createManualAsyncIterable<T>()
    streams.push(next)
    return next.iterable
  })
  return { streams, streamFactory }
}

describe('useResource', () => {
  // Each test renders into one root in a fresh container.
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

  // mount creates the test's container and root.
  const mount = () => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  }

  // update runs fn inside act, then lets flushes promise rounds run.
  const update = async (fn: () => void, flushes = 1) => {
    await act(async () => {
      fn()
      for (let i = 0; i < flushes; i++) await flush()
    })
  }

  // render renders element into the root inside act.
  const render = (element: React.ReactElement, flushes = 1) =>
    update(() => root?.render(element), flushes)

  // click clicks the rendered button.
  const click = () =>
    update(() =>
      container
        ?.querySelector('button')
        ?.dispatchEvent(new MouseEvent('click', { bubbles: true })),
    )

  // view reads an attribute of the rendered element.
  const view = (name: string) =>
    container?.firstElementChild?.getAttribute(name)

  it('retries released SDK resources by default', async () => {
    // Render a handle from a factory that counts its calls.
    const client = new FakeResourceClient()
    let nextId = 1
    const factory = vi.fn(async () => buildHandle(nextId++))
    mount()
    await render(
      React.createElement(
        ResourcesProvider,
        { client: client as never },
        React.createElement(TestHandle, { factory }),
      ),
    )
    expect(view('data-handle-id')).toBe('1')

    // A server release builds a new handle.
    await update(() =>
      client.emit({ resourceId: 1, reason: 'server-released' }),
    )
    expect(view('data-handle-id')).toBe('2')
    expect(factory).toHaveBeenCalledTimes(2)
  })

  it('allows opting out of release-triggered retries', async () => {
    // Render a handle that does not retry on release.
    const client = new FakeResourceClient()
    let nextId = 1
    const factory = vi.fn(async () => buildHandle(nextId++))
    mount()
    await render(
      React.createElement(
        ResourcesProvider,
        { client: client as never },
        React.createElement(TestHandle, {
          factory,
          retryOnReleasedResource: false,
        }),
      ),
    )
    expect(view('data-handle-id')).toBe('1')

    // A server release keeps the first handle.
    await update(() =>
      client.emit({ resourceId: 1, reason: 'server-released' }),
    )
    expect(view('data-handle-id')).toBe('1')
    expect(factory).toHaveBeenCalledTimes(1)
  })

  it('retries only failed parents', async () => {
    // Render a child under a healthy parent.
    const parentRetry = vi.fn()
    const factory = vi.fn(async (value: string) => `child:${value}`)
    const parent = (value: string | null, error: Error | null) =>
      React.createElement(TestRetryChild, {
        parent: { value, loading: false, error, retry: parentRetry },
        factory,
      })
    mount()
    await render(parent('ok', null))
    expect(factory).toHaveBeenCalledTimes(1)

    // A retry reruns only the child.
    await click()
    expect(parentRetry).not.toHaveBeenCalled()
    expect(factory).toHaveBeenCalledTimes(2)

    // Under a failed parent, a retry reaches the parent.
    await render(parent(null, new Error('parent failed')))
    await click()
    expect(parentRetry).toHaveBeenCalledTimes(1)
  })

  it('aborts child work when its SDK parent is released', async () => {
    // Start child work that never finishes under an SDK parent.
    const client = new FakeResourceClient()
    const parent: Resource<FakeSDKHandle> = {
      value: buildHandle(1),
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
    mount()
    await render(
      React.createElement(
        ResourcesProvider,
        { client: client as never },
        React.createElement(TestChildResource, {
          parent,
          factory: childFactory,
        }),
      ),
    )
    expect(childFactory).toHaveBeenCalledOnce()
    expect(childSignal?.aborted).toBe(false)

    // Releasing the parent aborts the child at once.
    await act(async () => {
      client.emit({ resourceId: 1, reason: 'client-released' })
      expect(childSignal?.aborted).toBe(true)
    })
  })

  it('keeps the previous resource value visible while a dependency reload is pending', async () => {
    // Load the first version.
    const { pending, factory } = deferredFactory<string>()
    mount()
    await render(React.createElement(TestValue, { factory, version: 1 }))
    await update(() => pending.get(1)?.resolve('value-1'))
    expect(view('data-value')).toBe('value-1')
    expect(view('data-loading')).toBe('false')

    // The pending reload keeps the first value.
    await render(React.createElement(TestValue, { factory, version: 2 }))
    expect(view('data-value')).toBe('value-1')
    expect(view('data-loading')).toBe('true')

    // The second value replaces it.
    await update(() => pending.get(2)?.resolve('value-2'))
    expect(view('data-value')).toBe('value-2')
    expect(view('data-loading')).toBe('false')
  })

  it('suppresses stale async completions after a dependency reload', async () => {
    // Start the first version and replace it before it loads.
    const { pending, factory } = deferredFactory<string>()
    mount()
    await render(React.createElement(TestValue, { factory, version: 1 }))
    expect(view('data-loading')).toBe('true')
    expect(view('data-value')).toBe('')
    await render(React.createElement(TestValue, { factory, version: 2 }))

    // The second version loads.
    await update(() => pending.get(2)?.resolve('value-2'))
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('value-2')

    // The first version's late result is ignored.
    await update(() => pending.get(1)?.resolve('stale-value-1'))
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('value-2')
    expect(factory).toHaveBeenCalledTimes(2)
  })

  it('keeps the previous streamed value visible while the parent reloads', async () => {
    // Stream from the first parent version.
    const { pending, factory } = deferredFactory<{ version: number }>()
    mount()
    await render(React.createElement(TestStreamValue, { factory, version: 1 }))
    await update(() => pending.get(1)?.resolve({ version: 1 }), 2)
    expect(view('data-value')).toBe('stream-1')
    expect(view('data-loading')).toBe('false')

    // The reloading parent keeps the first streamed value.
    await render(React.createElement(TestStreamValue, { factory, version: 2 }))
    expect(view('data-value')).toBe('stream-1')
    expect(view('data-loading')).toBe('true')

    // The second parent's stream replaces it.
    await update(() => pending.get(2)?.resolve({ version: 2 }), 2)
    expect(view('data-value')).toBe('stream-2')
    expect(view('data-loading')).toBe('false')
  })

  it('marks the stream loading while a parent replacement is still pending', async () => {
    // Stream from the first parent version.
    const { streams, streamFactory } = manualStreams<string>()
    const renderParent = (version: number, loading: boolean) =>
      render(
        React.createElement(TestStreamingFromParent, {
          parent: { value: { version }, loading, error: null, retry: () => {} },
          streamFactory,
        }),
      )
    mount()
    await renderParent(1, false)
    await update(() => streams[0].push('stream-1'))
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('stream-1')

    // A loading replacement marks the stream loading without reopening it.
    await renderParent(2, true)
    expect(view('data-loading')).toBe('true')
    expect(view('data-value')).toBe('stream-1')
    expect(streamFactory).toHaveBeenCalledTimes(1)

    // The loaded replacement opens a new stream, loading until it yields.
    await renderParent(2, false)
    expect(view('data-loading')).toBe('true')
    expect(streamFactory).toHaveBeenCalledTimes(2)
    await update(() => streams[1].push('stream-2'))
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('stream-2')
  })

  it('ignores stale stream errors while a parent replacement is in flight', async () => {
    // Render parent versions over manual streams.
    const { pending, factory } = deferredFactory<{ version: number }>()
    const { streams, streamFactory } = manualStreams<string>()
    const renderVersion = (version: number) =>
      render(
        React.createElement(TestStreamValue, {
          factory,
          version,
          streamFactory,
        }),
      )

    // Stream from the first parent version.
    mount()
    await renderVersion(1)
    await update(() => pending.get(1)?.resolve({ version: 1 }), 2)
    await update(() => streams[0].push('stream-1'))
    expect(view('data-value')).toBe('stream-1')
    expect(view('data-error')).toBe('')

    // The old stream's failure during the replacement shows no error.
    await renderVersion(2)
    await update(() => {
      streams[0].fail(new Error('released handle'))
      pending.get(2)?.resolve({ version: 2 })
    }, 2)
    expect(view('data-loading')).toBe('true')
    expect(view('data-value')).toBe('stream-1')
    expect(view('data-error')).toBe('')

    // The new stream's value replaces the old one.
    await update(() => streams[1].push('stream-2'))
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('stream-2')
    expect(view('data-error')).toBe('')
  })

  it('retries a current stream abort without staying loading forever', async () => {
    // Open the first stream.
    const factory = vi.fn(async (version: number) => ({ version }))
    const { streams, streamFactory } = manualStreams<string>()
    mount()
    await render(
      React.createElement(TestStreamValue, {
        factory,
        version: 1,
        streamFactory,
      }),
      2,
    )
    expect(streamFactory).toHaveBeenCalledTimes(1)

    // An abort reopens the stream without an error.
    await update(() => streams[0].fail(new Error('ERR_RPC_ABORT')), 3)
    expect(streamFactory).toHaveBeenCalledTimes(2)
    expect(view('data-loading')).toBe('true')
    expect(view('data-error')).toBe('')

    // The reopened stream's value settles the view.
    await update(() => streams[1].push('stream-1-retry'))
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('stream-1-retry')
    expect(view('data-error')).toBe('')
  })

  it('surfaces repeated current stream aborts instead of hanging', async () => {
    // Open the first stream and abort it once.
    const factory = vi.fn(async (version: number) => ({ version }))
    const { streams, streamFactory } = manualStreams<string>()
    mount()
    await render(
      React.createElement(TestStreamValue, {
        factory,
        version: 1,
        streamFactory,
      }),
      2,
    )
    await update(() => streams[0].fail(new Error('ERR_RPC_ABORT')), 3)
    expect(streamFactory).toHaveBeenCalledTimes(2)

    // A second abort reports the error.
    await update(() => streams[1].fail(new Error('ERR_RPC_ABORT')), 2)
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('')
    expect(view('data-error')).toBe('ERR_RPC_ABORT')
  })

  it('settles as not loading when the parent resolves to null', async () => {
    // Render a stream under a parent that resolves to null.
    const factory = vi.fn(async (): Promise<{ version: number } | null> => null)
    mount()
    await render(
      React.createElement(TestStreamValue, {
        factory: factory as never,
        version: 1,
      }),
      2,
    )

    // The view settles empty.
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('')
    expect(view('data-error')).toBe('')
  })

  it('settles as not loading when an explicit parent has no terminal value', async () => {
    // Render a stream under a settled parent with no value.
    const streamFactory = vi.fn((version: number) =>
      streamValue(`stream-${version}`),
    )
    mount()
    await render(
      React.createElement(TestStreamingFromParent, {
        parent: { value: null, loading: false, error: null, retry: () => {} },
        streamFactory,
      }),
    )

    // The view settles empty without opening the stream.
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('')
    expect(view('data-error')).toBe('')
    expect(streamFactory).not.toHaveBeenCalled()
  })

  it('propagates mapped resource loading, error, and retry while mapping non-null values', async () => {
    // Map a loading, failed source with a value.
    let retryCalls = 0
    const retry = () => {
      retryCalls += 1
    }
    const mapValue = vi.fn((value: string) => `mapped:${value}`)
    mount()
    await render(
      React.createElement(TestMappedValue, {
        source: {
          value: 'source-value',
          loading: true,
          error: new Error('source failed'),
          retry,
        },
        mapValue,
      }),
    )

    // The mapped resource carries the source's state and mapped value.
    expect(view('data-loading')).toBe('true')
    expect(view('data-value')).toBe('mapped:source-value')
    expect(view('data-error')).toBe('source failed')
    expect(mapValue).toHaveBeenCalledTimes(1)
    expect(mapValue).toHaveBeenCalledWith('source-value')

    // A retry reaches the source.
    await click()
    expect(retryCalls).toBe(1)

    // A null source value is not mapped.
    mapValue.mockClear()
    await render(
      React.createElement(TestMappedValue, {
        source: { value: null, loading: false, error: null, retry },
        mapValue,
      }),
    )
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('')
    expect(view('data-error')).toBe('')
    expect(mapValue).not.toHaveBeenCalled()
  })

  it('keeps the current value when equals matches the next one', async () => {
    // Stream rows that equal each other by id.
    const stream = createManualAsyncIterable<Row>()
    const seen: Row[] = []
    mount()
    await render(
      React.createElement(TestStreamOptions, {
        streamFactory: () => stream.iterable,
        options: { equals: (a, b) => a.id === b.id },
        seen,
      }),
      2,
    )

    // An equal row keeps the first row's identity; a changed one replaces it.
    for (const row of [
      { id: 1, label: 'first' },
      { id: 1, label: 'same id' },
      { id: 2, label: 'second' },
    ]) {
      await update(() => stream.push(row))
    }
    expect(seen.map((row) => row.label)).toEqual(['first', 'second'])
  })

  it('reconnects a stream that fails after yielding', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      // Open a reconnecting stream and read its first value.
      const { streams, streamFactory } = manualStreams<Row>()
      mount()
      await render(
        React.createElement(TestStreamOptions, {
          streamFactory,
          options: { reconnect: true },
          seen: [],
        }),
        2,
      )
      await update(() => streams[0].push({ id: 1, label: 'first' }))

      // A failure keeps the value and reopens the stream after the backoff.
      await update(() => streams[0].fail(new Error('dropped')))
      expect(view('data-value')).toBe('first')
      expect(view('data-error')).toBe('')
      await act(async () => {
        await vi.advanceTimersByTimeAsync(100)
      })
      expect(streamFactory).toHaveBeenCalledTimes(2)

      // The reopened stream replaces the value.
      await update(() => streams[1].push({ id: 2, label: 'second' }))
      expect(view('data-value')).toBe('second')
      expect(view('data-loading')).toBe('false')
    } finally {
      vi.useRealTimers()
    }
  })

  it('reopens a stream that gives no first value by its deadline', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      // Open a stream that never yields.
      const { streams, streamFactory } = manualStreams<Row>()
      mount()
      await render(
        React.createElement(TestStreamOptions, {
          streamFactory,
          options: { firstValueTimeout: 1000 },
          seen: [],
        }),
        2,
      )
      expect(streamFactory).toHaveBeenCalledTimes(1)

      // At the deadline it closes and reopens after the backoff, loading.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1100)
      })
      expect(streamFactory).toHaveBeenCalledTimes(2)
      expect(view('data-loading')).toBe('true')
      expect(view('data-error')).toBe('')

      // The second deadline is twice the first.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500)
      })
      expect(streamFactory).toHaveBeenCalledTimes(2)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(800)
      })
      expect(streamFactory).toHaveBeenCalledTimes(3)

      // A value from the reopened stream settles it and stops the deadline.
      await update(() => streams[2].push({ id: 1, label: 'first' }))
      expect(view('data-value')).toBe('first')
      expect(view('data-loading')).toBe('false')
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_000)
      })
      expect(streamFactory).toHaveBeenCalledTimes(3)
    } finally {
      vi.useRealTimers()
    }
  })

  it('reports a reconnecting stream that fails before yielding', async () => {
    // Open a reconnecting stream that fails at once.
    const stream = createManualAsyncIterable<Row>()
    mount()
    await render(
      React.createElement(TestStreamOptions, {
        streamFactory: () => stream.iterable,
        options: { reconnect: true },
        seen: [],
      }),
      2,
    )
    await update(() => stream.fail(new Error('refused')))

    // The error shows instead of an endless reconnect.
    expect(view('data-error')).toBe('refused')
  })
})
