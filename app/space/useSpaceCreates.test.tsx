import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { Toaster } from '@s4wave/web/ui/toaster.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { useSpaceCreates, type CreatePlan } from './useSpaceCreates.js'

// plan creates a layout. Each create has its own toast id, so the tests
// share one key.
const plan: CreatePlan = {
  name: 'Layout',
  objectKey: 'layout-1',
  objectType: 'alpha/object-layout',
  opId: 'op',
  opData: new Uint8Array([1]),
  persistent: false,
}

// fakeWorld fails each send with the next of failures, then succeeds; a
// repeat finds no object at the planned key.
function fakeWorld(failures: Error[]) {
  const applyWorldOp = vi.fn(() => {
    const err = failures.shift()
    return err ? Promise.reject(err) : Promise.resolve({ seqno: 1n })
  })
  const listObjects = vi.fn<IWorldState['listObjects']>(() =>
    Promise.resolve({ objects: [] }),
  )
  return { applyWorldOp, listObjects } as unknown as IWorldState & {
    applyWorldOp: typeof applyWorldOp
    listObjects: typeof listObjects
  }
}

// Harness mounts the owner with the real sonner Toaster, so a toast's
// action runs through sonner's own click handling.
function Harness(props: {
  world: IWorldState | null
  onReady: (create: () => void) => void
}) {
  const creates = useSpaceCreates('space-1', props.world, () => undefined)
  props.onReady(() => creates.create('Layout', () => Promise.resolve(plan)))
  return <Toaster />
}

function mount(world: IWorldState | null) {
  let create = () => undefined as void
  const onReady = (next: () => void) => (create = next)
  const view = render(<Harness world={world} onReady={onReady} />)
  return {
    create: () => act(() => create()),
    setWorld: (next: IWorldState | null) =>
      view.rerender(<Harness world={next} onReady={onReady} />),
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
    const view = mount(world)
    view.create()

    fireEvent.click(await screen.findByRole('button', { name: 'Try again' }))
    await past()

    expect(world.applyWorldOp).toHaveBeenCalledTimes(2)
    expect(screen.getByText('Could not create Layout')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Dismiss' })).toBeDefined()
  })

  it('closes a refused create with its Dismiss, freeing its key', async () => {
    const world = fakeWorld([new Error('offline')])
    world.listObjects
      .mockResolvedValueOnce({
        objects: [{ objectKey: plan.objectKey, typeId: 'other/type' }],
      })
      .mockResolvedValue({ objects: [] })
    const view = mount(world)
    view.create()
    fireEvent.click(await screen.findByRole('button', { name: 'Try again' }))
    await past()

    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    await past()
    expect(screen.queryByText('Could not create Layout')).toBeNull()

    // The key is free, so the next create of it sends rather than joining.
    view.create()
    await past()
    expect(world.applyWorldOp).toHaveBeenCalledTimes(2)
  })

  it('shows a Try again pressed while the world reloads as waiting, and Cancel returns to it', async () => {
    const world = fakeWorld([new Error('offline')])
    const view = mount(world)
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
