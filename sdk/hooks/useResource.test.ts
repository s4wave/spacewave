import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act } from 'react-dom/test-utils'
import { createRoot, type Root } from 'react-dom/client'
import { ResourcesProvider } from './ResourcesContext.js'
import { useResource } from './useResource.js'
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
})
