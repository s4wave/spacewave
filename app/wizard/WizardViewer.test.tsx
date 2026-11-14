import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CanvasInitOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { ClusterCreateOp } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import type { ObjectWizard } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'
import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'

const mocks = vi.hoisted(() => ({
  navigateToObjects: vi.fn(),
  setOpenMenu: vi.fn(),
  openCommand: vi.fn(),
  t: undefined as unknown as WizardTestWorld,
  updateState: vi.fn().mockResolvedValue({}),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  closeShellTab: vi.fn(),
}))

let currentState = {
  step: 0,
  targetTypeId: 'canvas',
  targetKeyPrefix: 'canvas/',
  name: 'Demo Canvas',
}

let currentWizards: ObjectWizard[] = [
  {
    typeId: 'canvas',
    displayName: 'Canvas',
    createOpId: 'space/world/init-canvas',
    keyPrefix: 'canvas/',
  },
]
let currentPeerId = ''
// watchedObject is what the world watch of the wizard object reports: the
// object while it exists, null once it is gone.
let watchedObject: {
  value: boolean | null
  loading: boolean
  error: Error | null
} = { value: true, loading: false, error: null }

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

vi.mock('@s4wave/web/sync/useObjectQuery.js', () => ({
  useObjectQuery: () => ({ ...watchedObject, retry: vi.fn() }),
}))

vi.mock('../ShellTabContext.js', () => ({
  useTabId: () => 'tab-1',
  useShellTabs: () => ({ closeShellTab: mocks.closeShellTab }),
  useShellTabsIfPresent: () => ({ closeShellTab: mocks.closeShellTab }),
}))

vi.mock('../space/useObjectWizards.js', () => ({
  useObjectWizards: () => ({
    value: { wizards: currentWizards },
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
import { wizardTestWorld, type WizardTestWorld } from './wizard-test-world.js'

describe('WizardViewer', () => {
  const mockSpace = {}

  function renderViewer() {
    return render(viewer())
  }

  function viewer() {
    return (
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
            value: mocks.t.world,
            loading: false,
            error: null,
            retry: vi.fn(),
          }}
          spaceWorld={mocks.t.world}
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
      </SpaceContext.Provider>
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
    watchedObject = { value: true, loading: false, error: null }
    vi.clearAllMocks()
    mocks.t = wizardTestWorld(['wizard/test-canvas'], '', keysWorld([]))
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllEnvs()
    localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
  })

  it('keeps the wizard name local while editing and persists before create', async () => {
    const user = userEvent.setup()
    renderViewer()

    const input = screen.getByPlaceholderText('Enter a name...')
    await waitFor(() => {
      expect((input as HTMLInputElement).value).toBe('Demo Canvas')
    })
    await user.clear(input)
    await user.type(input, 'Configured Canvas')
    await waitFor(() => {
      expect((input as HTMLInputElement).value).toBe('Configured Canvas')
    })

    expect(mocks.updateState).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => {
      expect(mocks.updateState).toHaveBeenCalledWith({
        name: 'Configured Canvas',
      })
    })
  })

  it('finalizes a test-type wizard by creating the target object, deleting the wizard, and navigating', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: /create/i }))

    // The canvas and the wizard delete committed together.
    await waitFor(() => expect(mocks.navigateToObjects).toHaveBeenCalled())
    expect(mocks.t.senders.get('space/world/init-canvas')).toBe('')
    const opData = mocks.t.opData.get('space/world/init-canvas')
    const decoded = CanvasInitOp.fromBinary(opData ?? new Uint8Array())
    expect(decoded.objectKey).toBe('demo-canvas-1')
    expect(mocks.t.state).toEqual({
      objects: new Set(),
      indexPath: '',
      creates: ['space/world/init-canvas'],
    })
    expect(mocks.navigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Created Demo Canvas')
  })

  it('finalizes a forge cluster wizard with an empty peer id and session sender', async () => {
    const user = userEvent.setup()
    currentState = {
      step: 0,
      targetTypeId: 'forge/cluster',
      targetKeyPrefix: 'forge/cluster/',
      name: 'test-cluster',
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

    await waitFor(() => expect(mocks.navigateToObjects).toHaveBeenCalled())
    expect(mocks.t.senders.get('forge/cluster/create')).toBe(currentPeerId)
    const opData = mocks.t.opData.get('forge/cluster/create')
    const decoded = ClusterCreateOp.fromBinary(opData ?? new Uint8Array())
    expect(decoded.clusterKey).toBe('test-cluster')
    expect(decoded.name).toBe('test-cluster')
    expect(decoded.peerId ?? '').toBe('')
    expect(mocks.t.state.objects.has('wizard/test-canvas')).toBe(false)
    expect(mocks.navigateToObjects).toHaveBeenCalledWith([decoded.clusterKey])
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Created test-cluster')
  })

  it('blocks an invalid Forge Cluster DNS-1123 name with actionable guidance', async () => {
    currentState = {
      step: 0,
      targetTypeId: 'forge/cluster',
      targetKeyPrefix: 'forge/cluster/',
      name: 'Invalid Cluster',
    }
    currentWizards = [
      {
        typeId: 'forge/cluster',
        displayName: 'Forge Cluster',
        createOpId: 'forge/cluster/create',
        keyPrefix: 'forge/cluster/',
      },
    ]

    renderViewer()

    expect(
      screen.getByText(
        'Use 1–63 lowercase letters, numbers, or hyphens; start and end with a letter or number.',
      ),
    ).toBeTruthy()
    expect(
      (screen.getByRole('button', { name: /create/i }) as HTMLButtonElement)
        .disabled,
    ).toBe(true)
    expect(mocks.t.calls).toEqual([])
  })

  it('does not finalize an experimental target wizard before browser opt-in', async () => {
    vi.stubEnv('DEV', false)
    const user = userEvent.setup()
    currentWizards = [
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
        experimental: true,
      },
    ]

    renderViewer()

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(mocks.t.calls).toEqual([])
  })

  it('finalizes an experimental target wizard after browser opt-in', async () => {
    vi.stubEnv('DEV', false)
    const user = userEvent.setup()
    setExperimentalCreatorsEnabled(true)
    currentWizards = [
      {
        typeId: 'canvas',
        displayName: 'Canvas',
        createOpId: 'space/world/init-canvas',
        keyPrefix: 'canvas/',
        experimental: true,
      },
    ]

    renderViewer()

    await user.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() =>
      expect(mocks.t.state.creates).toEqual(['space/world/init-canvas']),
    )
  })

  it('cancels by deleting the wizard object, then closes its tab', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: /delete wizard/i }))

    await waitFor(() =>
      expect(mocks.closeShellTab).toHaveBeenCalledWith('tab-1'),
    )
    expect(mocks.t.state.objects.has('wizard/test-canvas')).toBe(false)
    expect(mocks.openCommand).not.toHaveBeenCalled()
    expect(mocks.navigateToObjects).not.toHaveBeenCalled()
  })

  it('shows a failed Cancel beneath the bar and keeps the wizard', async () => {
    const user = userEvent.setup()
    mocks.t.failOn = 'commit'
    renderViewer()

    await user.click(screen.getByRole('button', { name: /delete wizard/i }))

    expect(
      await screen.findByText('Could not delete the wizard: commit failed'),
    ).toBeTruthy()
    expect(mocks.t.state.objects.has('wizard/test-canvas')).toBe(true)
    expect(mocks.closeShellTab).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /create/i })).toBeTruthy()
  })

  it('shows the gone screen, not a failure, when a Cancel finds the wizard gone', async () => {
    // Another tab finished first; this tab's watch has not caught up.
    const user = userEvent.setup()
    mocks.t.state.objects.delete('wizard/test-canvas')
    renderViewer()

    await user.click(screen.getByRole('button', { name: /delete wizard/i }))

    expect(await screen.findByText('Wizard deleted')).toBeTruthy()
    expect(screen.queryByText(/Could not delete the wizard/)).toBeNull()
    expect(mocks.t.calls).not.toContain('commit')
    expect(mocks.closeShellTab).not.toHaveBeenCalled()
  })

  it("hides Delete wizard while this view's Finish runs", async () => {
    const user = userEvent.setup()
    let finishCommit = () => {}
    mocks.t.commitGate = new Promise((resolve) => {
      finishCommit = resolve
    })
    renderViewer()
    expect(screen.getByRole('button', { name: /delete wizard/i })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => {
      expect(mocks.t.calls).toContain('delete')
    })
    expect(screen.queryByRole('button', { name: /delete wizard/i })).toBeNull()

    finishCommit()
    await waitFor(() => {
      expect(mocks.navigateToObjects).toHaveBeenCalledWith(['demo-canvas-1'])
    })
  })

  it('shows a deleted state with a close-tab action once the watched object is gone', async () => {
    const user = userEvent.setup()
    watchedObject = { value: null, loading: false, error: null }
    renderViewer()

    expect(screen.getByText('Wizard deleted')).toBeTruthy()
    expect(screen.queryByText('Loading wizard')).toBeNull()
    expect(screen.queryByRole('button', { name: /create/i })).toBeNull()

    await user.click(screen.getByRole('button', { name: /close tab/i }))
    expect(mocks.closeShellTab).toHaveBeenCalledWith('tab-1')
  })

  it('shows the deleted state when the wizard state never streamed', () => {
    watchedObject = { value: null, loading: false, error: null }
    currentState = undefined as never
    renderViewer()

    expect(screen.getByText('Wizard deleted')).toBeTruthy()
    expect(screen.queryByText('Loading wizard')).toBeNull()
  })

  it('does not show the deleted state while Finish removes the wizard', async () => {
    const user = userEvent.setup()
    let finishCommit = () => {}
    mocks.t.commitGate = new Promise((resolve) => {
      finishCommit = resolve
    })
    const view = renderViewer()

    // The watch can see the wizard gone before the commit call returns.
    await user.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => {
      expect(mocks.t.calls).toContain('delete')
    })
    watchedObject = { value: null, loading: false, error: null }
    view.rerender(viewer())
    expect(screen.queryByText('Wizard deleted')).toBeNull()

    finishCommit()
    await waitFor(() => {
      expect(mocks.navigateToObjects).toHaveBeenCalledWith(['demo-canvas-1'])
    })
  })

  it('keeps the input and shows Retry when Finish fails, then creates once', async () => {
    const user = userEvent.setup()
    mocks.t.failOn = 'delete'
    renderViewer()

    // The failed Finish wrote nothing and says why.
    await user.click(screen.getByRole('button', { name: /create/i }))
    expect(
      await screen.findByText('Could not finish: delete failed'),
    ).toBeTruthy()
    expect(mocks.t.state.creates).toEqual([])
    expect(mocks.t.state.objects.has('wizard/test-canvas')).toBe(true)
    expect(
      (screen.getByPlaceholderText('Enter a name...') as HTMLInputElement)
        .value,
    ).toBe('Demo Canvas')

    // Retry runs the whole Finish again and creates one canvas.
    mocks.t.failOn = undefined
    await user.click(screen.getByRole('button', { name: /retry/i }))
    await waitFor(() => {
      expect(mocks.navigateToObjects).toHaveBeenCalledWith(['demo-canvas-1'])
    })
    expect(mocks.t.state.creates).toEqual(['space/world/init-canvas'])
  })

  it('says nothing changed and shows Retry when a plugin times out', async () => {
    const user = userEvent.setup()
    mocks.t.timeoutOn = 'space/world/init-canvas'
    renderViewer()

    await user.click(screen.getByRole('button', { name: /create/i }))
    expect(
      await screen.findByText(
        'Could not finish: the plugin did not finish in time, so nothing was changed',
      ),
    ).toBeTruthy()
    expect(mocks.t.state.creates).toEqual([])
    expect(mocks.t.state.objects.has('wizard/test-canvas')).toBe(true)
    expect(screen.getByRole('button', { name: /retry/i })).toBeTruthy()
  })

  it('shows the gone state, with no Retry, when another tab removed the wizard', async () => {
    // Another tab finished first; this tab's watch has not caught up.
    const user = userEvent.setup()
    mocks.t.state.objects.delete('wizard/test-canvas')
    renderViewer()

    // The refused Finish goes straight to the gone state, whose only way out
    // is to close the tab.
    await user.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText('Wizard deleted')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull()
    expect(mocks.t.state.creates).toEqual([])
    await user.click(screen.getByRole('button', { name: 'Close tab' }))
    expect(mocks.closeShellTab).toHaveBeenCalledWith('tab-1')
  })

  it('keeps loading while the object watch has not settled', () => {
    watchedObject = { value: null, loading: true, error: null }
    currentState = undefined as never
    renderViewer()

    expect(screen.getByText('Loading wizard')).toBeTruthy()
    expect(screen.queryByText('Wizard deleted')).toBeNull()
  })
})
