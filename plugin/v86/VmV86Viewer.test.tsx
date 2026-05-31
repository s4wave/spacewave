import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SetV86StateOp, VmState, VmV86 } from '@s4wave/sdk/vm/v86.pb.js'

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    loadAddon = vi.fn()
    open = vi.fn()
    onData = vi.fn(() => ({ dispose: vi.fn() }))
    write = vi.fn()
    dispose = vi.fn()
  },
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit = vi.fn()
  },
}))

vi.mock('@s4wave/sdk/world/types/types.js', () => ({
  listObjectsWithType: vi.fn(() => Promise.resolve([])),
}))

import VmV86Viewer from './VmV86Viewer.js'

function disposable<T extends object>(
  value: T,
): T & {
  [Symbol.dispose]: () => void
} {
  return {
    ...value,
    [Symbol.dispose]: () => undefined,
  }
}

function buildWorld(vm: VmV86) {
  const applyWorldOp = vi.fn(() => Promise.resolve({ sysErr: false }))
  const world = {
    getObject: vi.fn(() =>
      Promise.resolve(
        disposable({
          accessWorldState: vi.fn(() =>
            Promise.resolve(
              disposable({
                getBlock: vi.fn(() =>
                  Promise.resolve({
                    found: true,
                    data: VmV86.toBinary(vm),
                  }),
                ),
              }),
            ),
          ),
        }),
      ),
    ),
    lookupGraphQuads: vi.fn(() => Promise.resolve({ quads: [] })),
    applyWorldOp,
  }
  return { world, applyWorldOp }
}

describe('VmV86Viewer', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'BroadcastChannel',
      class {
        onmessage: ((event: MessageEvent) => void) | null = null
        constructor(public name: string) {}
        postMessage = vi.fn()
        close = vi.fn()
      },
    )
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  it('shows the stored runtime error and resets before start is available', async () => {
    const { world, applyWorldOp } = buildWorld(
      VmV86.create({
        state: VmState.VmState_ERROR,
        errorMessage: 'missing V86 BIOS image',
      }),
    )

    render(
      <VmV86Viewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: { objectKey: 'vm/v86/test', objectType: 'vm/v86' },
          },
        }}
        worldState={{
          value: world,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(await screen.findByText('Runtime error')).toBeDefined()
    expect(screen.getByText('missing V86 BIOS image')).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Start' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Reset' }))

    await waitFor(() => {
      expect(applyWorldOp).toHaveBeenCalled()
    })
    const [, opData] = applyWorldOp.mock.calls[0] as [string, Uint8Array]
    const op = SetV86StateOp.fromBinary(opData)
    expect(op.objectKey).toBe('vm/v86/test')
    expect(op.state ?? VmState.VmState_STOPPED).toBe(VmState.VmState_STOPPED)
  })
})
