import { render, screen, cleanup, act } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { WatchListenerStatusResponse } from '@s4wave/sdk/root/root.pb.js'

// Shared mock watch function; each test replaces the stream it returns
// so useWatchStateRpc drives the hook through a fresh emission
// sequence.
const streamRef: {
  current: null | AsyncIterable<WatchListenerStatusResponse>
} = vi.hoisted(() => ({ current: null }))

// Fake Root resource handle exposed via useRootResource.
const rootRef: { current: unknown } = vi.hoisted(() => ({ current: null }))

vi.mock('@s4wave/web/hooks/useRootResource.js', () => ({
  useRootResource: () => ({ value: rootRef.current }),
}))

// Passthrough shim for useWatchStateRpc. It drives a simple async
// iteration over the stream handed in via streamRef so component
// renders see the emitted values.
vi.mock('@aptre/bldr-react', async () => {
  const { useEffect, useState } = await import('react')
  return {
    useWatchStateRpc: <T,>(
      rpc: (req: unknown, signal: AbortSignal) => AsyncIterable<T> | null,
      req: unknown,
    ): T | null => {
      const [value, setValue] = useState<T | null>(null)
      useEffect(() => {
        const abort = new AbortController()
        const stream = rpc(req, abort.signal)
        if (!stream) return () => abort.abort()
        ;(async () => {
          for await (const resp of stream) {
            if (abort.signal.aborted) return
            setValue(resp)
          }
        })()
        return () => abort.abort()
      }, [rpc, req])
      return value
    },
  }
})

import { useListenerStatus } from './useListenerStatus.js'

// makeStream wraps a fixed list of emissions as an AsyncIterable and
// leaves an unresolved promise at the end so the hook behaves like a
// live stream that has not been torn down yet.
function makeStream(
  emissions: WatchListenerStatusResponse[],
): AsyncIterable<WatchListenerStatusResponse> {
  return {
    [Symbol.asyncIterator]() {
      let i = 0
      return {
        async next() {
          if (i < emissions.length) {
            return { value: emissions[i++], done: false as const }
          }
          // Block forever to mimic a live stream.
          await new Promise(() => {})
          return { value: undefined, done: true as const }
        },
      }
    },
  }
}

function Harness() {
  const status = useListenerStatus()
  if (!status) return <div data-testid="loading">loading</div>
  return (
    <div data-testid="status">
      <span data-testid="socket-path">{status.socketPath}</span>
      <span data-testid="listening">{String(status.listening)}</span>
      <span data-testid="clients">{String(status.connectedClients)}</span>
    </div>
  )
}

describe('useListenerStatus', () => {
  beforeEach(() => {
    rootRef.current = {
      watchListenerStatus: () => streamRef.current,
    }
  })

  afterEach(() => {
    cleanup()
    streamRef.current = null
    rootRef.current = null
  })

  it('returns null while the stream has not emitted', () => {
    streamRef.current = makeStream([])
    render(<Harness />)
    expect(screen.getByTestId('loading').textContent).toBe('loading')
  })

  it('maps the first emission to the ListenerStatus shape', async () => {
    streamRef.current = makeStream([
      {
        socketPath: '/run/spacewave.sock',
        listening: true,
        connectedClients: 2,
      },
    ])
    render(<Harness />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(screen.getByTestId('socket-path').textContent).toBe(
      '/run/spacewave.sock',
    )
    expect(screen.getByTestId('listening').textContent).toBe('true')
    expect(screen.getByTestId('clients').textContent).toBe('2')
  })

  it('fills proto defaults when fields are omitted', async () => {
    streamRef.current = makeStream([{}])
    render(<Harness />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(screen.getByTestId('socket-path').textContent).toBe('')
    expect(screen.getByTestId('listening').textContent).toBe('false')
    expect(screen.getByTestId('clients').textContent).toBe('0')
  })

  it('returns null while the root resource is unavailable', () => {
    rootRef.current = null
    streamRef.current = makeStream([
      { socketPath: '/ignored', listening: true, connectedClients: 0 },
    ])
    render(<Harness />)
    expect(screen.getByTestId('loading').textContent).toBe('loading')
  })
})
