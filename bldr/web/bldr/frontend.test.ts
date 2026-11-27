// @vitest-environment node
import { pushable } from 'it-pushable'
import { afterEach, expect, it, vi } from 'vitest'

import { Event, Session } from '../../frontend/frontend.pb.js'
import { FrontendResource } from './frontend.js'

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

/** snapshot supplies the ordered compiler capability at attachment time. */
function snapshot(id = 'recovery', sequence = 0n): Event {
  return Event.create({
    session: Session.create({
      id,
      routePrefix: `/b/fe/${id}/`,
      entrypoints: ['App.tsx'],
    }),
    sequence,
  })
}

it('resumes a dropped stream without replacing the unchanged compiler', async () => {
  // Control only the stream interruption and retain the real resource and
  // retry.
  vi.useFakeTimers()
  vi.spyOn(console, 'debug').mockImplementation(() => {})
  const streams = [
    pushable<Event>({ objectMode: true }),
    pushable<Event>({ objectMode: true }),
  ]
  const watch = vi.fn((_request, signal?: AbortSignal) => {
    const stream = streams[watch.mock.calls.length - 1]!
    signal?.addEventListener('abort', () => stream.end(), { once: true })
    return stream
  })
  const invalidated = vi.fn()
  const frontend = new FrontendResource(
    { Watch: watch, Send: async () => ({}) },
    invalidated,
  )
  try {
    // An ordered update is accepted before the connection drops.
    streams[0]!.push(snapshot())
    await frontend.getSession()
    const received: unknown[] = []
    const delivered = Promise.withResolvers<void>()
    await frontend.connect({
      onMessage(payload) {
        received.push(payload)
        if ((payload as { type: string }).type === 'update') delivered.resolve()
      },
    })
    streams[0]!.push(
      Event.create({
        sequence: 1n,
        payload: '{"type":"custom","event":"before"}',
      }),
    )
    streams[0]!.end(new Error('connection dropped'))
    await vi.advanceTimersByTimeAsync(500)

    // The matching snapshot resumes delivery through the existing Vite handlers.
    expect(watch).toHaveBeenCalledTimes(2)
    streams[1]!.push(snapshot('recovery', 1n))
    streams[1]!.push(
      Event.create({ sequence: 2n, payload: '{"type":"update","updates":[]}' }),
    )
    await delivered.promise
    expect(received).toEqual([
      { type: 'connected' },
      { type: 'custom', event: 'before' },
      { type: 'update', updates: [] },
    ])
    expect(invalidated).not.toHaveBeenCalled()
    expect(globalThis.__bldrFrontends?.get('recovery')).toBe(frontend)
  } finally {
    frontend.release()
  }
})

it.each([
  ['missed updates', snapshot('recovery', 2n)],
  ['configuration replacement', snapshot('replacement', 0n)],
])('invalidates after %s on reconnection', async (_reason, replacement) => {
  // Attach to an admitted graph before interrupting its stream.
  vi.useFakeTimers()
  vi.spyOn(console, 'debug').mockImplementation(() => {})
  const streams = [
    pushable<Event>({ objectMode: true }),
    pushable<Event>({ objectMode: true }),
  ]
  const watch = vi.fn((_request, signal?: AbortSignal) => {
    const stream = streams[watch.mock.calls.length - 1]!
    signal?.addEventListener('abort', () => stream.end(), { once: true })
    return stream
  })
  const invalidated = Promise.withResolvers<Error>()
  const frontend = new FrontendResource(
    { Watch: watch, Send: async () => ({}) },
    invalidated.resolve,
  )
  try {
    streams[0]!.push(snapshot())
    await frontend.getSession()
    streams[0]!.end()
    await vi.advanceTimersByTimeAsync(500)

    // A new graph or missed event must discard the old document attachment.
    streams[1]!.push(replacement)
    expect((await invalidated.promise).message).toBe(
      'Bldr frontend session changed',
    )
    expect(globalThis.__bldrFrontends?.has('recovery')).toBe(false)
    await expect(frontend.getSession()).rejects.toThrow('attachment closed')
  } finally {
    frontend.release()
  }
})

it('invalidates a gap on an otherwise live stream', async () => {
  // Admit the graph and attach Vite's consumer.
  const stream = pushable<Event>({ objectMode: true })
  const invalidated = Promise.withResolvers<Error>()
  const frontend = new FrontendResource(
    {
      Watch: (_request, signal) => {
        signal?.addEventListener('abort', () => stream.end(), { once: true })
        return stream
      },
      Send: async () => ({}),
    },
    invalidated.resolve,
  )
  try {
    stream.push(snapshot())
    await frontend.getSession()
    const received = vi.fn()
    await frontend.connect({ onMessage: received })

    // Never deliver an update whose predecessor was not observed.
    stream.push(Event.create({ sequence: 2n, payload: '{"type":"update"}' }))
    expect((await invalidated.promise).message).toContain('history has a gap')
    expect(received).toHaveBeenCalledExactlyOnceWith({ type: 'connected' })
  } finally {
    frontend.release()
  }
})

it('releases a pending attachment and cancels reconnect work', async () => {
  // Release before readiness so both waiters and the stream are canceled.
  vi.useFakeTimers()
  const stream = pushable<Event>({ objectMode: true })
  let signal: AbortSignal | undefined
  const watch = vi.fn((_request, current?: AbortSignal) => {
    signal = current
    current?.addEventListener('abort', () => stream.end(), { once: true })
    return stream
  })
  const invalidated = vi.fn()
  const frontend = new FrontendResource(
    { Watch: watch, Send: async () => ({}) },
    invalidated,
  )

  // Wait for the initial attempt, then release its unresolved admission.
  const waiting = frontend.getSession()
  await vi.advanceTimersByTimeAsync(0)
  frontend.release()
  await expect(waiting).rejects.toThrow('attachment closed')

  // Advancing the existing retry clock cannot resurrect a released resource.
  await vi.advanceTimersByTimeAsync(5000)
  expect(signal?.aborted).toBe(true)
  expect(watch).toHaveBeenCalledTimes(1)
  expect(invalidated).not.toHaveBeenCalled()
})
