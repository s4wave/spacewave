import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { z } from 'zod'

import { defineApp, type AppSource } from '../../sdk/sync/app.js'
import { SyncError } from '../../sdk/sync/errors.js'
import { defineQuery } from '../../sdk/sync/query.js'
import { useAppMutation, useAppQuery } from './app-hooks.js'

const app = defineApp(
  {
    id: 'test/hooks',
    version: 1,
    collections: { counts: z.number() },
    mutations: {
      add: { input: z.object({ amount: z.number() }), output: z.number() },
    },
  },
  { add: async (_context, { amount }) => amount },
)
const query = defineQuery(app.schema, {
  name: 'count',
  input: z.object({ key: z.string() }),
  evaluate: async ({ collection }, { key }) =>
    (await collection('counts').get(key)) ?? 0,
})
type Source = AppSource<typeof app.schema>

describe('application hooks', () => {
  it('retries the original intent and hides acceptance from a replaced source', async () => {
    const mutate = vi
      .fn()
      .mockRejectedValueOnce(
        new SyncError('UNCERTAIN', 'Retry the same request'),
      )
      .mockResolvedValueOnce(3)
    const source = { mutate } as unknown as Source
    let controls!: ReturnType<typeof useAppMutation<typeof app.schema, 'add'>>
    function View({ source }: { source: Source | null }) {
      controls = useAppMutation(source, 'add')
      return <div>{controls.state.status}</div>
    }

    const container = document.createElement('div')
    const root = createRoot(container)
    try {
      await act(async () => root.render(<View source={source} />))
      const input = { amount: 3 }
      await act(async () => {
        controls.submit(input)
        expect(() => controls.submit(input)).toThrow('still pending')
        input.amount = 99
      })
      expect(container.textContent).toBe('error')
      const requestId = mutate.mock.calls[0][2].requestId
      expect(mutate.mock.calls[0][1]).toEqual({ amount: 3 })

      await act(async () => controls.retry())
      expect(container.textContent).toBe('accepted')
      expect(mutate.mock.calls[1][2].requestId).toBe(requestId)
      expect(mutate.mock.calls[1][1]).toEqual({ amount: 3 })

      await act(async () => root.render(<View source={null} />))
      expect(container.textContent).toBe('idle')
      expect(mutate).toHaveBeenCalledTimes(2)
    } finally {
      await act(async () => root.unmount())
    }
  })

  it('keeps equivalent query arguments subscribed and releases on source replacement', async () => {
    const released: number[] = []
    const watched = vi.fn()
    const source = (value: number): Source =>
      ({
        collection: () => {
          throw new Error('Query reads belong to the source')
        },
        mutate: async () => {
          throw new Error('This source only serves queries')
        },
        watch: async function* (_definition, _args, signal) {
          watched(value)
          try {
            yield { status: 'current', seqno: 1n, value } as never
            await new Promise<void>((resolve) => {
              if (signal?.aborted) resolve()
              else
                signal?.addEventListener('abort', () => resolve(), {
                  once: true,
                })
            })
          } finally {
            released.push(value)
          }
        },
      }) as Source
    const first = source(1)
    const second = source(2)
    const renders: string[] = []
    function View({ source }: { source: Source | null }) {
      const result = useAppQuery(source, query, { key: 'count' })
      const text =
        result.status === 'current' ? String(result.value) : result.status
      renders.push(text)
      return <div>{text}</div>
    }

    const container = document.createElement('div')
    const root = createRoot(container)
    try {
      await act(async () => root.render(<View source={first} />))
      expect(container.textContent).toBe('1')
      await act(async () => root.render(<View source={first} />))
      expect(watched).toHaveBeenCalledTimes(1)

      renders.length = 0
      await act(async () => root.render(<View source={second} />))
      expect(renders[0]).toBe('pending')
      expect(container.textContent).toBe('2')
      expect(released).toEqual([1])

      await act(async () => root.render(<View source={null} />))
      expect(container.textContent).toBe('pending')
      expect(released).toEqual([1, 2])
    } finally {
      await act(async () => root.unmount())
    }
  })
})
