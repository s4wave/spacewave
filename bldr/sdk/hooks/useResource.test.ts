import React, { act, useEffect } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRoot, type Root } from 'react-dom/client'
import { ResourcesProvider } from './ResourcesContext.js'
import { useResource, type Resource } from './useResource.js'
import { resourceOf } from './resourceTransitionState.js'
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

/** lastOf returns the last value a failed resource keeps, or null. */
function lastOf<T>(resource: Resource<T>): T | null {
  return resource.error ? resource.last : null
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
    {
      'data-handle-id': resource.value?.id ?? 0,
      'data-last-id': lastOf(resource)?.id ?? 0,
      'data-error': resource.error?.message ?? '',
    },
    React.createElement(
      'button',
      { type: 'button', onClick: resource.retry },
      'retry',
    ),
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

function TestParentChild(props: {
  parent: Resource<string>
  factory: (value: string) => Promise<string>
}) {
  const resource = useResource(
    props.parent,
    async (value) => props.factory(value),
    [],
  )

  return React.createElement('div', {
    'data-value': resource.value ?? '',
    'data-last': lastOf(resource) ?? '',
    'data-error': resource.error?.message ?? '',
  })
}

function TestValue(props: {
  factory: (version: number) => Promise<string>
  version: number
  renders?: string[]
}) {
  const resource = useResource(
    async () => props.factory(props.version),
    [props.version],
  )
  props.renders?.push(
    `${props.version}:${resource.value ?? ''}:${lastOf(resource) ?? ''}:${resource.loading}`,
  )

  return React.createElement(
    'div',
    {
      'data-loading': String(resource.loading),
      'data-value': resource.value ?? '',
      'data-last': lastOf(resource) ?? '',
      'data-error': resource.error?.message ?? '',
    },
    React.createElement(
      'button',
      { type: 'button', onClick: resource.retry },
      'retry',
    ),
  )
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
    'data-last': lastOf(resource) ?? '',
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
      'data-last': lastOf(resource) ?? '',
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
  version?: number
}) {
  // Stream rows from a trivial parent; version is the stream's only dep.
  const parent = useResource(async () => ({}), [])
  const resource = useStreamingResource(
    parent,
    () => props.streamFactory(),
    [props.version],
    props.options,
  )

  // Record each value the consumer receives.
  const { seen } = props
  useEffect(() => {
    if (resource.value && seen.at(-1) !== resource.value) {
      seen.push(resource.value)
    }
  }, [resource.value, seen])

  return React.createElement(
    'div',
    {
      'data-loading': String(resource.loading),
      'data-value': resource.value?.label ?? '',
      'data-last': lastOf(resource)?.label ?? '',
      'data-error': resource.error?.message ?? '',
    },
    React.createElement(
      'button',
      { type: 'button', onClick: resource.retry },
      'retry',
    ),
  )
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

/** settledByHand returns a factory whose nth call the test resolves or fails. */
function settledByHand() {
  const results: Array<(value: string) => void> = []
  const errors: Array<(err: Error) => void> = []
  const factory = vi.fn(
    (_version: number) =>
      new Promise<string>((resolve, reject) => {
        results.push(resolve)
        errors.push(reject)
      }),
  )
  return { results, errors, factory }
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
        parent: resourceOf({ value, loading: false, error, retry: parentRetry }),
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

  it('keeps a child failed over its last through its parent recovering', async () => {
    // A child reads a value under a ready parent.
    const results: Array<(value: string) => void> = []
    const factory = vi.fn(
      (_value: string) =>
        new Promise<string>((resolve) => {
          results.push(resolve)
        }),
    )
    const parent = (value: string | null, error: Error | null) =>
      React.createElement(TestParentChild, {
        parent: resourceOf({ value, loading: false, error, retry: () => {} }),
        factory,
      })
    mount()
    await render(parent('p1', null))
    await update(() => results[0]('child-1'))
    expect(view('data-value')).toBe('child-1')

    // The parent fails, then recovers with a new value: the child's run
    // starts failed over its last, never blank.
    await render(parent(null, new Error('parent failed')))
    expect(view('data-last')).toBe('child-1')
    await render(parent('p2', null))
    expect(factory).toHaveBeenCalledTimes(2)
    expect(view('data-last')).toBe('child-1')
    expect(view('data-error')).toBe('parent failed')

    // The run's value returns it to ready.
    await update(() => results[1]('child-2'))
    expect(view('data-value')).toBe('child-2')
    expect(view('data-error')).toBe('')
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

  it('drops the previous value on the first render of new dependencies', async () => {
    // Load the first version.
    const { pending, factory } = deferredFactory<string>()
    const renders: string[] = []
    mount()
    await render(React.createElement(TestValue, { factory, version: 1, renders }))
    await update(() => pending.get(1)?.resolve('value-1'))
    expect(view('data-value')).toBe('value-1')
    expect(view('data-loading')).toBe('false')

    // No render under the second version shows the first value.
    await render(React.createElement(TestValue, { factory, version: 2, renders }))
    expect(view('data-value')).toBe('')
    expect(view('data-loading')).toBe('true')
    const second = renders.filter((r) => r.startsWith('2:'))
    expect(second.length).toBeGreaterThan(0)
    expect(second.every((r) => r === '2:::true')).toBe(true)

    // The second value arrives.
    await update(() => pending.get(2)?.resolve('value-2'))
    expect(view('data-value')).toBe('value-2')
    expect(view('data-loading')).toBe('false')
  })

  it('keeps a failed value as last and drops it on new dependencies', async () => {
    // Load the first version, then fail its retried read.
    const { results, errors, factory } = settledByHand()
    const renders: string[] = []
    mount()
    await render(React.createElement(TestValue, { factory, version: 1, renders }))
    await update(() => results[0]('value-1'))
    await click()
    await update(() => errors[1](new Error('read failed')))

    // The failure reads null, keeping the value as last.
    expect(view('data-value')).toBe('')
    expect(view('data-last')).toBe('value-1')
    expect(view('data-error')).toBe('read failed')
    expect(view('data-loading')).toBe('false')

    // New dependencies clear value and last on their first render.
    renders.length = 0
    await render(
      React.createElement(TestValue, { factory, version: 2, renders }),
    )
    expect(renders.length).toBeGreaterThan(0)
    expect(renders.every((r) => r === '2:::true')).toBe(true)
    expect(view('data-error')).toBe('')
    expect(view('data-loading')).toBe('true')

    // A failure under the new dependencies keeps nothing from the old ones.
    await update(() => errors[2](new Error('second failed')))
    expect(view('data-error')).toBe('second failed')
    expect(view('data-last')).toBe('')
  })

  it('recovers from a failure on Retry', async () => {
    // Load a value, then fail a retried read.
    const { results, errors, factory } = settledByHand()
    mount()
    await render(React.createElement(TestValue, { factory, version: 1 }))
    await update(() => results[0]('value-1'))
    await click()
    await update(() => errors[1](new Error('read failed')))
    expect(view('data-last')).toBe('value-1')
    expect(view('data-error')).toBe('read failed')

    // Retry reads again quietly: the failure stays over the kept value, never
    // blanking it, until the new value settles.
    await click()
    expect(factory).toHaveBeenCalledTimes(3)
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('')
    expect(view('data-last')).toBe('value-1')
    expect(view('data-error')).toBe('read failed')
    await update(() => results[2]('value-2'))
    expect(view('data-value')).toBe('value-2')
    expect(view('data-last')).toBe('')
    expect(view('data-loading')).toBe('false')
  })

  it('keeps no released handle as last after a failure', async () => {
    // Load a handle, then fail the retried read.
    let calls = 0
    const factory = vi.fn(async () => {
      calls += 1
      if (calls > 1) throw new Error('mount failed')
      return buildHandle(1)
    })
    mount()
    await render(React.createElement(TestHandle, { factory }))
    expect(view('data-handle-id')).toBe('1')

    // The retry disposed the handle, so the failure keeps nothing.
    await click()
    expect(view('data-error')).toBe('mount failed')
    expect(view('data-handle-id')).toBe('0')
    expect(view('data-last-id')).toBe('0')
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
    // Map a source that loads again with a value.
    let retryCalls = 0
    const retry = () => {
      retryCalls += 1
    }
    const mapValue = vi.fn((value: string) => `mapped:${value}`)
    mount()
    await render(
      React.createElement(TestMappedValue, {
        source: resourceOf({
          value: 'source-value',
          loading: true,
          error: null,
          retry,
        }),
        mapValue,
      }),
    )
    expect(view('data-loading')).toBe('true')
    expect(view('data-value')).toBe('mapped:source-value')
    expect(mapValue).toHaveBeenCalledWith('source-value')

    // A failed source maps its last value and passes its error.
    await render(
      React.createElement(TestMappedValue, {
        source: resourceOf({
          value: null,
          last: 'source-value',
          loading: false,
          error: new Error('source failed'),
          retry,
        }),
        mapValue,
      }),
    )
    expect(view('data-loading')).toBe('false')
    expect(view('data-value')).toBe('')
    expect(view('data-last')).toBe('mapped:source-value')
    expect(view('data-error')).toBe('source failed')

    // A retry reaches the source.
    await click()
    expect(retryCalls).toBe(1)

    // A null source value is not mapped.
    mapValue.mockClear()
    await render(
      React.createElement(TestMappedValue, {
        source: resourceOf({ value: null, loading: false, error: null, retry }),
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
  it('fails a child stream over its last value when its parent fails', async () => {
    // Stream a value under a ready parent.
    const { streams, streamFactory } = manualStreams<string>()
    const renderParent = (parent: Resource<{ version: number }>) =>
      render(React.createElement(TestStreamingFromParent, { parent, streamFactory }))
    const ready = resourceOf({
      value: { version: 1 },
      loading: false,
      error: null,
      retry: () => {},
    })
    mount()
    await renderParent(ready)
    await update(() => streams[0].push('stream-1'))
    expect(view('data-value')).toBe('stream-1')

    // The parent's failure reads null, with the stream's data as last.
    await renderParent(
      resourceOf({
        value: null,
        loading: false,
        error: new Error('parent failed'),
        retry: () => {},
      }),
    )
    expect(view('data-value')).toBe('')
    expect(view('data-last')).toBe('stream-1')
    expect(view('data-error')).toBe('parent failed')
    expect(streamFactory).toHaveBeenCalledTimes(1)

    // The parent's recovery opens the stream again, failed over its last
    // until the stream yields, never blank.
    await renderParent(ready)
    expect(streamFactory).toHaveBeenCalledTimes(2)
    expect(view('data-last')).toBe('stream-1')
    expect(view('data-error')).toBe('parent failed')
    await update(() => streams[1].push('stream-2'))
    expect(view('data-value')).toBe('stream-2')
    expect(view('data-error')).toBe('')
  })

  it('keeps the previous value while new dependencies load only with keepWhileLoading', async () => {
    const { streams, streamFactory } = manualStreams<Row>()
    const options = { keepWhileLoading: true }
    mount()
    await render(
      React.createElement(TestStreamOptions, {
        streamFactory,
        options,
        seen: [],
        version: 1,
      }),
      2,
    )
    await update(() => streams[0].push({ id: 1, label: 'first' }))

    // New dependencies show the previous value while they load.
    await render(
      React.createElement(TestStreamOptions, {
        streamFactory,
        options,
        seen: [],
        version: 2,
      }),
    )
    expect(view('data-value')).toBe('first')
    expect(view('data-loading')).toBe('true')

    // A failure of the new stream drops it and keeps no last.
    await update(() => streams[1].fail(new Error('dropped')))
    expect(view('data-value')).toBe('')
    expect(view('data-last')).toBe('')
    expect(view('data-error')).toBe('dropped')
  })

  it('reports a reconnecting stream failed at the backoff cap', async () => {
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
      const reopen = () =>
        act(async () => {
          await vi.advanceTimersByTimeAsync(5000)
        })

      // Failures below the cap keep the value, loading, with no error.
      for (let i = 0; i < 6; i++) {
        await update(() => streams[i].fail(new Error('dropped')))
        expect(view('data-value')).toBe('first')
        expect(view('data-loading')).toBe('true')
        expect(view('data-error')).toBe('')
        await reopen()
      }

      // The failure at the cap reports failed, with the value as last.
      await update(() => streams[6].fail(new Error('dropped')))
      expect(view('data-value')).toBe('')
      expect(view('data-last')).toBe('first')
      expect(view('data-error')).toBe('dropped')
      expect(view('data-loading')).toBe('false')

      // It keeps reopening without leaving the failed state.
      await reopen()
      expect(streamFactory).toHaveBeenCalledTimes(8)
      expect(view('data-error')).toBe('dropped')
      expect(view('data-loading')).toBe('false')

      // Retry reopens at once, still failed over the kept value so it never
      // blanks; a value settles it.
      await click()
      expect(streamFactory).toHaveBeenCalledTimes(9)
      expect(view('data-last')).toBe('first')
      expect(view('data-error')).toBe('dropped')
      await update(() => streams[8].push({ id: 2, label: 'second' }))
      expect(view('data-value')).toBe('second')
      expect(view('data-last')).toBe('')
      expect(view('data-error')).toBe('')
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('resourceOf', () => {
  it('reports a value beside an error as failed with no value', () => {
    const retry = () => {}
    const r = resourceOf({
      value: 'stale',
      last: 'kept',
      loading: true,
      error: new Error('read failed'),
      retry,
    })
    expect(r).toEqual({
      value: null,
      last: 'kept',
      loading: false,
      error: new Error('read failed'),
      retry,
    })
  })
})
