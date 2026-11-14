import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { Toaster } from '@s4wave/web/ui/toaster.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { useSpaceCreates, type CreatePlan } from './useSpaceCreates.js'

// planFor plans a layout at key; each test uses its own key, since sonner
// keeps every toast id it has seen in one module-wide store.
const planFor = (objectKey: string): CreatePlan => ({
  name: 'Layout',
  objectKey,
  objectType: 'alpha/object-layout',
  opId: 'op',
  opData: new Uint8Array([1]),
  persistent: false,
})

// fakeWorld fails each send with the next of failures, then succeeds; a
// repeat finds no object at the planned key.
function fakeWorld(failures: Error[]) {
  const applyWorldOp = vi.fn(() => {
    const err = failures.shift()
    return err ? Promise.reject(err) : Promise.resolve({ seqno: 1n })
  })
  const listObjects = vi.fn(() => Promise.resolve({ objects: [] }))
  return { applyWorldOp, listObjects } as unknown as IWorldState & {
    applyWorldOp: typeof applyWorldOp
  }
}

// Harness mounts the owner with the real sonner Toaster, so a toast's
// action runs through sonner's own click handling.
function Harness(props: {
  world: IWorldState | null
  objectKey: string
  onReady: (create: () => void) => void
}) {
  const creates = useSpaceCreates('space-1', props.world, () => undefined)
  props.onReady(() =>
    creates.create('Layout', () => Promise.resolve(planFor(props.objectKey))),
  )
  return <Toaster />
}

function mount(world: IWorldState | null, objectKey: string) {
  let create = () => undefined as void
  const onReady = (next: () => void) => (create = next)
  const view = render(
    <Harness world={world} objectKey={objectKey} onReady={onReady} />,
  )
  return {
    create: () => act(() => create()),
    setWorld: (next: IWorldState | null) =>
      view.rerender(
        <Harness world={next} objectKey={objectKey} onReady={onReady} />,
      ),
  }
}

// past waits beyond sonner's 200 ms removal of a toast whose action it
// deleted, with margin.
const past = () => act(() => new Promise((resolve) => setTimeout(resolve, 700)))

afterEach(() => {
  cleanup()
})

describe('useSpaceCreates with the real Toaster', () => {
  it('keeps the failure and its Try again when the retry fails at once', async () => {
    const world = fakeWorld([new Error('offline'), new Error('offline')])
    const view = mount(world, 'layout-fast')
    view.create()

    fireEvent.click(await screen.findByRole('button', { name: 'Try again' }))
    await past()

    expect(world.applyWorldOp).toHaveBeenCalledTimes(2)
    expect(screen.getByText('Could not create Layout')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()
  })

  it('shows a Try again pressed while the world reloads as waiting, and Cancel returns to it', async () => {
    const world = fakeWorld([new Error('offline')])
    const view = mount(world, 'layout-wait')
    view.create()
    const retry = await screen.findByRole('button', { name: 'Try again' })

    view.setWorld(null)
    fireEvent.click(retry)
    await past()

    expect(
      screen.getByText('Waiting for the Space to reconnect.'),
    ).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await past()
    expect(screen.getByText('Could not create Layout')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()

    // The cancelled press does not send when the world returns.
    view.setWorld(world)
    await past()
    expect(world.applyWorldOp).toHaveBeenCalledTimes(1)
  })
})
