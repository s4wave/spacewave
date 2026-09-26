import { act, renderHook, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { useGetValueRpc, useMemoEqualGetter } from './hooks.js'

it('ignores a superseded response that resolves after a newer one', async () => {
  const resolvers = new Map<string, (value: string) => void>()
  // The RPC deliberately ignores cancellation to exercise the consumer.
  const getValue = (req: string) =>
    new Promise<string>((resolve) => {
      resolvers.set(req, resolve)
    })
  const hook = renderHook(
    ({ req }) => useGetValueRpc(getValue, req, Object.is),
    { initialProps: { req: 'old' } },
  )
  await waitFor(() => expect(resolvers.has('old')).toBe(true))

  hook.rerender({ req: 'new' })
  await waitFor(() => expect(resolvers.has('new')).toBe(true))

  await act(async () => resolvers.get('new')?.('new value'))
  await waitFor(() => expect(hook.result.current).toBe('new value'))
  await act(async () => resolvers.get('old')?.('old value'))
  expect(hook.result.current).toBe('new value')

  hook.unmount()
})

it('recomputes the derived value when a non-null input changes', () => {
  const getter = vi.fn((value: { id: number }) => value.id * 10)
  const checkEqual = (a: { id: number }, b: { id: number }) => a.id === b.id
  const hook = renderHook(
    ({ value }) => useMemoEqualGetter(value, getter, checkEqual),
    { initialProps: { value: { id: 1 } } },
  )
  expect(hook.result.current).toBe(10)

  hook.rerender({ value: { id: 1 } })
  expect(hook.result.current).toBe(10)

  hook.rerender({ value: { id: 2 } })
  expect(hook.result.current).toBe(20)
})
