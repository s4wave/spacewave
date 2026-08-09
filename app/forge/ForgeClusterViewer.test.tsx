import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import { ForgeClusterViewer } from './ForgeClusterViewer.js'
import type { ForgeClusterSnapshot } from './useForgeClusterSnapshot.js'

const mockVisibleWizardTypeSet = new Set(['forge/job'])
const mockClusterSnapshot: {
  snapshot: ForgeClusterSnapshot
  loading: boolean
  error: Error | null
} = {
  snapshot: { jobs: [], tasks: [], passes: [], executions: [], workers: [] },
  loading: false,
  error: null as Error | null,
}

vi.mock('@s4wave/web/forge/ForgeViewerShell.js', () => ({
  ForgeViewerShell: ({
    tabs,
    headerStatus,
  }: {
    tabs?: Array<{ id: string; label: string; content: React.ReactNode }>
    headerStatus?: React.ReactNode
  }) => {
    const [activeTab, setActiveTab] = React.useState(tabs?.[0]?.id)
    return (
      <div data-testid="forge-viewer-shell">
        {tabs?.map((tab) => (
          <button key={tab.id} onClick={() => setActiveTab(tab.id)}>
            {tab.label}
          </button>
        ))}
        {headerStatus}
        {tabs?.find((tab) => tab.id === activeTab)?.content}
      </div>
    )
  },
}))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlockData: () => ({
    name: 'Build Cluster',
    peerId: '12D3KooWCluster',
  }),
}))

vi.mock('@s4wave/web/forge/useForgeLinkedEntities.js', () => ({
  useForgeLinkedEntities: () => ({
    entities: [],
    loading: false,
  }),
}))

vi.mock('./useForgeClusterSnapshot.js', () => ({
  useForgeClusterSnapshot: () => mockClusterSnapshot,
}))

vi.mock('../space/useVisibleObjectWizardTypeSet.js', () => ({
  useVisibleObjectWizardTypeSet: () => mockVisibleWizardTypeSet,
}))

describe('ForgeClusterViewer', () => {
  const mockSpaceWorld = {
    applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  }
  const mockNavigateToObjects = vi.fn()

  function renderViewer() {
    return render(
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
        navigateToObjects={mockNavigateToObjects}
        buildObjectUrls={vi.fn()}
        navigateToSubPath={vi.fn()}
      >
        <ForgeClusterViewer
          objectInfo={{
            info: {
              case: 'worldObjectInfo',
              value: {
                objectKey: 'forge/cluster/main',
                objectType: 'forge/cluster',
              },
            },
          }}
          worldState={{
            value: {} as never,
            loading: false,
            error: null,
            retry: vi.fn(),
          }}
          objectState={{} as never}
        />
      </SpaceContainerContext.Provider>,
    )
  }

  afterEach(() => {
    cleanup()
    mockClusterSnapshot.snapshot = {
      jobs: [],
      tasks: [],
      passes: [],
      executions: [],
      workers: [],
    }
    mockClusterSnapshot.error = null
    mockVisibleWizardTypeSet.clear()
    mockVisibleWizardTypeSet.add('forge/job')
    vi.clearAllMocks()
  })

  it('opens the job wizard with the current cluster preselected', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'Jobs' }))
    await user.click(screen.getByRole('button', { name: /create job/i }))

    expect(mockSpaceWorld.applyWorldOp).toHaveBeenCalledTimes(1)
    const [opTypeId, opData] = mockSpaceWorld.applyWorldOp.mock.calls[0] as [
      string,
      Uint8Array,
    ]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/job-1')
    expect(decoded.wizardTypeId).toBe('wizard/forge/job')
    expect(decoded.targetTypeId).toBe('forge/job')
    expect(decoded.initialStep).toBe(1)

    const config = ForgeJobCreateOp.fromBinary(decoded.initialConfigData)
    expect(config.clusterKey).toBe('forge/cluster/main')
    expect(mockNavigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('keeps a published snapshot truncation alert visible across tabs', async () => {
    const user = userEvent.setup()
    mockClusterSnapshot.snapshot = {
      jobs: [],
      tasks: [],
      passes: [],
      executions: [],
      workers: [
        {
          objectKey: 'forge/worker/one',
          data: { name: 'Published worker' },
          clusterKeys: ['forge/cluster/main'],
          keypairKeys: [],
          peerIds: [],
        },
      ],
    }
    mockClusterSnapshot.error = new Error(
      'Forge graph snapshot exceeds the 200-edge limit for forge/cluster/main.',
    )

    renderViewer()

    expect(screen.getByRole('alert').textContent).toContain(
      'exceeds the 200-edge limit',
    )
    await user.click(screen.getByRole('button', { name: 'Workers' }))
    expect(screen.getByText('Published worker')).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain(
      'exceeds the 200-edge limit',
    )
  })

  it('hides the create-job affordance when forge jobs are experimental', async () => {
    mockVisibleWizardTypeSet.clear()

    renderViewer()
    await userEvent.click(screen.getByRole('button', { name: 'Jobs' }))

    expect(screen.queryByRole('button', { name: /create job/i })).toBeNull()
  })
})
