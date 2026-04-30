import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { ForgeDashboardViewer } from './ForgeDashboardViewer.js'

const mockVisibleWizardTypeSet = new Set(['forge/cluster', 'forge/job'])
const mockContents = {
  setProcessBinding: vi.fn().mockResolvedValue(undefined),
}
const mockSpaceWorld = {
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
}
const mockNavigateToObjects = vi.fn()

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockContents,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: {
      processBindings: [
        {
          objectKey: 'forge/worker/session',
          typeId: 'forge/worker',
          approved: false,
        },
      ],
    },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContentsContext: {
    useContext: () => mockContents,
  },
}))

vi.mock('./useForgeDashboardActivity.js', () => ({
  useForgeDashboardActivity: () => ({
    entries: [
      {
        id: 'activity-1',
        objectKey: 'forge/execution/run-1',
        typeId: 'forge/execution',
        title: 'Execution COMPLETE',
        detail: 'noop execution complete',
        timestamp: new Date('2026-04-17T12:00:00Z'),
      },
    ],
    loading: false,
  }),
}))

vi.mock('../space/useVisibleObjectWizardTypeSet.js', () => ({
  useVisibleObjectWizardTypeSet: () => mockVisibleWizardTypeSet,
}))

vi.mock('@s4wave/web/forge/ForgeViewerShell.js', () => ({
  ForgeViewerShell: ({
    tabs,
    actions,
  }: {
    tabs?: Array<{ id: string; content: React.ReactNode }>
    actions?: Array<{ label: string; onClick: () => void }>
  }) => (
    <div data-testid="forge-viewer-shell">
      {tabs?.map((tab) => (
        <div key={tab.id}>{tab.content}</div>
      ))}
      {actions?.map((action) => (
        <button key={action.label} type="button" onClick={action.onClick}>
          {action.label}
        </button>
      ))}
    </div>
  ),
}))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlockData: () => ({
    name: 'Forge Dashboard',
  }),
}))

vi.mock('@s4wave/web/forge/useForgeLinkedEntities.js', () => ({
  useForgeLinkedEntities: () => ({
    entities: [
      {
        objectKey: 'forge/cluster/main',
        typeId: 'forge/cluster',
      },
      {
        objectKey: 'forge/worker/session',
        typeId: 'forge/worker',
      },
    ],
    loading: false,
  }),
}))

describe('ForgeDashboardViewer', () => {
  function renderViewer() {
    return render(
      <SpaceContainerContext.Provider
        spaceId="space-1"
        spaceState={{ ready: true } as never}
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
        <ForgeDashboardViewer
          objectInfo={{
            info: {
              case: 'worldObjectInfo',
              value: {
                objectKey: 'forge/dashboard',
                objectType: 'spacewave/forge/dashboard',
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
    mockVisibleWizardTypeSet.clear()
    mockVisibleWizardTypeSet.add('forge/cluster')
    mockVisibleWizardTypeSet.add('forge/job')
    vi.clearAllMocks()
  })

  it('starts the quickstart worker from the explicit affordance', async () => {
    const user = userEvent.setup()

    renderViewer()

    const [startWorkerButton] = screen.getAllByRole('button', {
      name: /start worker/i,
    })
    if (!startWorkerButton) {
      throw new Error('expected start worker action')
    }

    await user.click(startWorkerButton)

    expect(mockContents.setProcessBinding).toHaveBeenCalledWith(
      'forge/worker/session',
      'forge/worker',
      true,
    )
  })

  it('opens cluster and job wizards from the dashboard action bar', async () => {
    const user = userEvent.setup()
    renderViewer()

    const [createClusterButton] = screen.getAllByRole('button', {
      name: /create cluster/i,
    })
    const [createJobButton] = screen.getAllByRole('button', {
      name: /create job/i,
    })
    if (!createClusterButton || !createJobButton) {
      throw new Error('expected dashboard create actions')
    }

    await user.click(createClusterButton)
    await user.click(createJobButton)

    expect(mockSpaceWorld.applyWorldOp).toHaveBeenCalledTimes(2)

    const [clusterOpTypeId, clusterOpData] = mockSpaceWorld.applyWorldOp.mock
      .calls[0] as [string, Uint8Array]
    expect(clusterOpTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const clusterOp = CreateWizardObjectOp.fromBinary(clusterOpData)
    expect(clusterOp.objectKey).toBe('wizard/cluster-1')
    expect(clusterOp.wizardTypeId).toBe('wizard/forge/cluster')
    expect(clusterOp.targetTypeId).toBe('forge/cluster')
    expect(clusterOp.targetKeyPrefix).toBe('forge/cluster/')

    const [jobOpTypeId, jobOpData] = mockSpaceWorld.applyWorldOp.mock
      .calls[1] as [string, Uint8Array]
    expect(jobOpTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)
    const jobOp = CreateWizardObjectOp.fromBinary(jobOpData)
    expect(jobOp.objectKey).toBe('wizard/job-1')
    expect(jobOp.wizardTypeId).toBe('wizard/forge/job')
    expect(jobOp.targetTypeId).toBe('forge/job')
    expect(jobOp.initialStep).toBe(1)

    const initialConfigData = jobOp.initialConfigData
    if (!initialConfigData) {
      throw new Error('expected initial job config data')
    }
    const config = ForgeJobCreateOp.fromBinary(initialConfigData)
    expect(config.clusterKey).toBe('forge/cluster/main')
    expect(mockNavigateToObjects).toHaveBeenNthCalledWith(1, [
      clusterOp.objectKey,
    ])
    expect(mockNavigateToObjects).toHaveBeenNthCalledWith(2, [jobOp.objectKey])
  })

  it('renders recent activity entries on the dashboard', () => {
    renderViewer()

    expect(screen.getAllByText('Execution COMPLETE').length).toBeGreaterThan(0)
    expect(
      screen.getAllByText('noop execution complete').length,
    ).toBeGreaterThan(0)
    expect(
      screen.getAllByText('2026-04-17T12:00:00.000Z').length,
    ).toBeGreaterThan(0)
  })

  it('hides create actions when forge creators are not visible', () => {
    mockVisibleWizardTypeSet.clear()

    renderViewer()

    expect(screen.queryByRole('button', { name: /create cluster/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /create job/i })).toBeNull()
  })
})
