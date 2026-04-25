import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CanvasInitOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { ClusterCreateOp } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

const mocks = vi.hoisted(() => ({
  navigateToObjects: vi.fn(),
  setOpenMenu: vi.fn(),
  openCommand: vi.fn(),
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  updateState: vi.fn().mockResolvedValue({}),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

let currentState = {
  step: 0,
  targetTypeId: 'canvas',
  targetKeyPrefix: 'canvas/',
  name: 'Demo Canvas',
}

let currentWizards = [
  {
    typeId: 'canvas',
    displayName: 'Canvas',
    createOpId: 'space/world/init-canvas',
    keyPrefix: 'canvas/',
  },
]
let currentPeerId = ''

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      updateState: mocks.updateState,
    },
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: currentState,
  }),
}))

vi.mock('@s4wave/web/hooks/usePromise.js', () => ({
  usePromise: () => ({
    data: currentWizards,
  }),
}))

vi.mock('@s4wave/web/hooks/useSessionInfo.js', () => ({
  useSessionInfo: () => ({
    peerId: currentPeerId,
  }),
}))

vi.mock('@s4wave/web/frame/bottom-bar-context.js', () => ({
  useBottomBarSetOpenMenu: () => mocks.setOpenMenu,
}))

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => mocks.openCommand,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
}))

import { WizardViewer } from './WizardViewer.js'

describe('WizardViewer', () => {
  const mockSpace = {
    listWizards: vi.fn(),
  }

  const mockSpaceWorld = {
    applyWorldOp: mocks.applyWorldOp,
    deleteObject: mocks.deleteObject,
  }

  function renderViewer() {
    return render(
      <SpaceContext.Provider
        resource={{
          value: mockSpace as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      >
        <SpaceContainerContext.Provider
          spaceId="space-1"
          spaceState={{ ready: true }}
          spaceWorldResource={{
            value: mockSpaceWorld as never,
            loading: false,
            error: null,
            retry: vi.fn(),
          }}
          spaceWorld={mockSpaceWorld as never}
          navigateToRoot={vi.fn()}
          navigateToObjects={mocks.navigateToObjects}
          buildObjectUrls={vi.fn()}
          navigateToSubPath={vi.fn()}
        >
          <WizardViewer
            objectInfo={{
              info: {
                case: 'worldObjectInfo',
                value: {
                  objectKey: 'wizard/test-canvas',
                  objectType: 'wizard/test',
                },
              },
            }}
            worldState={{
              value: {} as never,
              loading: false,
              error: null,
              retry: vi.fn(),
            }}
          />
        </SpaceContainerContext.Provider>
      </SpaceContext.Provider>,
    )
  }

  beforeEach(() => {
    currentState = {
      step: 0,
      targetTypeId: 'canvas',
      targetKeyPrefix: 'canvas/',
      name: 'Demo Canvas',
    }
    currentWizards = [
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
      },
    ]
    currentPeerId = ''
    vi.clearAllMocks()
  })

  afterEach(() => {
    cleanup()
  })

  it('keeps the wizard name local while editing and persists before create', async () => {
    const user = userEvent.setup()
    renderViewer()

    const input = screen.getByPlaceholderText('Enter a name...')
    fireEvent.change(input, { target: { value: 'Configured Canvas' } })

    expect(mocks.updateState).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(mocks.updateState).toHaveBeenCalledWith({
      name: 'Configured Canvas',
    })
  })

  it('finalizes a test-type wizard by creating the target object, deleting the wizard, and navigating', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(mocks.applyWorldOp).toHaveBeenCalledTimes(1)
    const [opTypeId, opData, sender] = mocks.applyWorldOp.mock.calls[0] as [
      string,
      Uint8Array,
      string,
    ]
    expect(opTypeId).toBe('space/world/init-canvas')
    expect(sender).toBe('')

    const decoded = CanvasInitOp.fromBinary(opData)
    expect(decoded.objectKey).toMatch(/^canvas-\d+$/)
    expect(mocks.deleteObject).toHaveBeenCalledWith('wizard/test-canvas')
    expect(mocks.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Created Demo Canvas')
  })

  it('finalizes a forge cluster wizard with an empty peer id and session sender', async () => {
    const user = userEvent.setup()
    currentState = {
      step: 0,
      targetTypeId: 'forge/cluster',
      targetKeyPrefix: 'forge/cluster/',
      name: 'Test Cluster',
    }
    currentWizards = [
      {
        typeId: 'forge/cluster',
        displayName: 'Forge Cluster',
        createOpId: 'forge/cluster/create',
        keyPrefix: 'forge/cluster/',
      },
    ]
    currentPeerId = '12D3KooWTestPeerID'

    renderViewer()

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(mocks.applyWorldOp).toHaveBeenCalledTimes(1)
    const [opTypeId, opData, sender] = mocks.applyWorldOp.mock.calls[0] as [
      string,
      Uint8Array,
      string,
    ]
    expect(opTypeId).toBe('forge/cluster/create')
    expect(sender).toBe(currentPeerId)

    const decoded = ClusterCreateOp.fromBinary(opData)
    expect(decoded.clusterKey).toMatch(/^cluster-\d+$/)
    expect(decoded.name).toBe('Test Cluster')
    expect(decoded.peerId ?? '').toBe('')
    expect(mocks.deleteObject).toHaveBeenCalledWith('wizard/test-canvas')
    expect(mocks.navigateToObjects).toHaveBeenCalledWith([decoded.clusterKey])
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Created Test Cluster')
  })

  it('cancels by deleting the wizard object without reopening the creation drawer', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: /delete wizard/i }))

    expect(mocks.deleteObject).toHaveBeenCalledWith('wizard/test-canvas')
    expect(mocks.openCommand).not.toHaveBeenCalled()
    expect(mocks.navigateToObjects).not.toHaveBeenCalled()
  })
})
