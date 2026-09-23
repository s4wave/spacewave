import { act, renderHook, waitFor } from '@testing-library/react'
import { pushable } from 'it-pushable'
import { expect, it } from 'vitest'

import { useWatchStateRpc } from './hooks.js'

it('hides the previous stream snapshot and ignores its late response', async () => {
  // These producers deliberately ignore cancellation to exercise the consumer.
  const first = pushable<string>({ objectMode: true })
  const second = pushable<string>({ objectMode: true })
  const firstWatch = () => first
  const secondWatch = () => second
  const hook = renderHook(
    ({ watch }) => useWatchStateRpc(watch, true, Object.is),
    { initialProps: { watch: firstWatch } },
  )
  await act(async () => first.push('first plugin'))
  await waitFor(() => expect(hook.result.current).toBe('first plugin'))

  // Replacing the connection hides old registrations before the new RPC replies.
  hook.rerender({ watch: secondWatch })
  expect(hook.result.current).toBeNull()
  await act(async () => first.push('late first plugin'))
  expect(hook.result.current).toBeNull()
  await act(async () => second.push('second plugin'))
  await waitFor(() => expect(hook.result.current).toBe('second plugin'))

  // Closing the hook releases its consumer even if the producer was delayed.
  hook.unmount()
  first.end()
  second.end()
})
