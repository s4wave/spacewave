import { renderHook } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import type { Root } from '@s4wave/sdk/root'

import { useDynamicRegistrations } from './useDynamicRegistrations.js'

interface Registration {
  id: string
  module: string
}

let snapshot: Registration[] = []
vi.mock('@aptre/bldr-react', () => ({ useWatchStateRpc: () => snapshot }))

const stream = async function* (): AsyncGenerator<Registration[]> {}
const getRegistrations = (value: Registration[] | null) => value ?? []
const equals = (a: Registration, b: Registration) =>
  a.id === b.id && a.module === b.module
const mapViewer = (registration: Registration) => ({
  ...registration,
  component: () => null,
})

it('retains unchanged viewer component types across plugin registration updates', () => {
  const root = {} as Root
  snapshot = [{ id: 'colors', module: '/immutable/one/Viewer.js' }]
  const hook = renderHook(
    ({ current }: { current: Root | null }) =>
      useDynamicRegistrations(
        current,
        stream,
        {},
        Object.is,
        Object.is,
        getRegistrations,
        mapViewer,
        equals,
      ),
    { initialProps: { current: root as Root | null } },
  )
  const original = hook.result.current[0].component

  // Registering a different plugin must not replace the mounted viewer type.
  snapshot = [
    { id: 'notes', module: '/immutable/notes/Viewer.js' },
    { id: 'colors', module: '/immutable/one/Viewer.js' },
  ]
  hook.rerender({ current: root })
  expect(hook.result.current[1].component).toBe(original)

  // A real immutable revision change replaces only that plugin's component.
  const notes = hook.result.current[0].component
  snapshot = [snapshot[0], { id: 'colors', module: '/immutable/two/Viewer.js' }]
  hook.rerender({ current: root })
  expect(hook.result.current[0].component).toBe(notes)
  expect(hook.result.current[1].component).not.toBe(original)

  // Removing the root drops its registrations before another watch can emit.
  hook.rerender({ current: null })
  expect(hook.result.current).toEqual([])
  hook.unmount()
})
