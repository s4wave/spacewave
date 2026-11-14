import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { SET_SPACE_INDEX_PATH_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { SetSpaceIndexPathOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useWizardState } from './useWizardState.js'

const h = vi.hoisted(() => ({
  updateState: vi.fn().mockResolvedValue({}),
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
}))

let currentConfigData = new Uint8Array()
let currentWizardName = 'Repo'

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      updateState: h.updateState,
    },
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: {
      step: 0,
      targetTypeId: 'git/repo',
      targetKeyPrefix: 'git/repo/',
      name: currentWizardName,
      configData: currentConfigData,
    },
  }),
}))

vi.mock('@s4wave/web/configtype/useConfigEditor.js', () => ({
  useConfigEditor: (
    _configTypeId: string,
    configData: Uint8Array | undefined,
    onConfigDataChange: (data: Uint8Array) => void,
  ) => {
    const value = new TextDecoder().decode(configData ?? new Uint8Array())
    return {
      element: (
        <input
          aria-label="Config value"
          value={value}
          onChange={(e) =>
            onConfigDataChange(new TextEncoder().encode(e.target.value))
          }
        />
      ),
      registration: undefined,
      value,
    }
  },
}))

describe('useWizardState', () => {
  afterEach(() => {
    currentConfigData = new Uint8Array()
    currentWizardName = 'Repo'
    cleanup()
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('keeps config edits local until the draft is persisted', async () => {
    const user = userEvent.setup()
    renderHarness()

    await user.type(screen.getByLabelText('Config value'), 'abc')

    expect(h.updateState).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /persist/i }))

    await waitFor(() => {
      expect(h.updateState).toHaveBeenCalledTimes(1)
    })
    expect(h.updateState).toHaveBeenCalledWith({
      configData: new TextEncoder().encode('abc'),
    })
  })

  it('does not restore the remote wizard name after a local clear has begun', async () => {
    const user = userEvent.setup()
    const view = renderHarness()
    const input = screen.getByLabelText('Wizard name') as HTMLInputElement

    await waitFor(() => {
      expect(input.value).toBe('Repo')
    })

    const nativeQueueMicrotask = globalThis.queueMicrotask
    const queuedMicrotasks: VoidFunction[] = []
    vi.stubGlobal('queueMicrotask', (callback: VoidFunction) => {
      queuedMicrotasks.push(callback)
    })
    currentWizardName = 'Repo from server'
    view.rerender(<HarnessTree />)
    vi.stubGlobal('queueMicrotask', nativeQueueMicrotask)

    await user.clear(input)
    await act(async () => {
      for (const run of queuedMicrotasks.splice(0)) run()
      await Promise.resolve()
    })

    expect(input.value).toBe('')

    await user.type(input, 'Renamed Repo')

    expect(input.value).toBe('Renamed Repo')
    expect(h.updateState).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /persist/i }))

    await waitFor(() => {
      expect(h.updateState).toHaveBeenCalledTimes(1)
    })
    expect(h.updateState).toHaveBeenCalledWith({
      name: 'Renamed Repo',
    })
  })

  it('moves an index naming the wizard before Cancel deletes it', async () => {
    // The Space index names this wizard, so Cancel repoints it first.
    const user = userEvent.setup()
    renderHarness()
    await user.click(screen.getByRole('button', { name: /cancel/i }))

    // The wizard is deleted after one index write.
    await waitFor(() => expect(h.deleteObject).toHaveBeenCalled())
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/git/repo/test')
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    expect(h.applyWorldOp.mock.calls[0]?.[0]).toBe(SET_SPACE_INDEX_PATH_OP_ID)

    // The write names the replacement and guards on the wizard's path.
    const opData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(opData instanceof Uint8Array)) throw new Error('expected op bytes')
    const op = SetSpaceIndexPathOp.fromBinary(opData)
    expect(op.indexPath).toBe('git/repo/next')
    expect(op.expectedIndexPath).toBe('wizard/git/repo/test')

    // The index write lands before the delete.
    expect(h.applyWorldOp.mock.invocationCallOrder[0]).toBeLessThan(
      h.deleteObject.mock.invocationCallOrder[0] ?? 0,
    )
  })
})

function renderHarness() {
  return render(<HarnessTree />)
}

function HarnessTree() {
  const spaceWorld = {
    applyWorldOp: h.applyWorldOp,
    deleteObject: h.deleteObject,
  }
  return (
    <SpaceContainerContext.Provider
      spaceId="space-1"
      spaceState={{
        ready: true,
        settings: { indexPath: 'wizard/git/repo/test' },
      }}
      spaceWorldResource={{
        value: spaceWorld as never,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
      spaceWorld={spaceWorld as never}
      navigateToRoot={vi.fn()}
      navigateToObjects={vi.fn()}
      buildObjectUrls={vi.fn()}
      navigateToSubPath={vi.fn()}
    >
      <Harness />
    </SpaceContainerContext.Provider>
  )
}

function Harness() {
  const ws = useWizardState(
    {
      objectInfo: {
        info: {
          case: 'worldObjectInfo',
          value: {
            objectKey: 'wizard/git/repo/test',
            objectType: 'wizard/git/repo',
          },
        },
      },
      worldState: {
        value: {} as never,
        loading: false,
        error: null,
        retry: vi.fn(),
      },
    },
    'git/repo',
  )
  return (
    <>
      <input
        aria-label="Wizard name"
        value={ws.localName}
        onChange={(e) => ws.handleUpdateName(e.target.value)}
      />
      {ws.configEditor.element}
      <button type="button" onClick={() => void ws.persistDraftState()}>
        Persist
      </button>
      <button
        type="button"
        onClick={() => void ws.handleCancel('git/repo/next')}
      >
        Cancel
      </button>
    </>
  )
}
