import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'

import { ForgeJobViewer } from './ForgeJobViewer.js'

const mockVisibleWizardTypeSet = new Set(['forge/task'])
const mockDependencyGraph = {
  edges: [
    { from: 'forge/task/a', to: 'forge/task/b', kind: 'subtask' as const },
  ],
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
    timestamp: new Date('2026-04-17T00:00:00Z'),
    jobState: 1,
  }),
}))

vi.mock('@s4wave/web/forge/useForgeLinkedEntities.js', () => ({
  useForgeLinkedEntities: () => ({
    entities: [
      {
        objectKey: 'forge/task/a',
        typeId: 'forge/task',
      },
      {
        objectKey: 'forge/task/b',
        typeId: 'forge/task',
      },
    ],
    loading: false,
  }),
}))

vi.mock('./useForgeDecodedLinkedEntities.js', () => ({
  useForgeDecodedLinkedEntities: () => ({
    items: [
      {
        entity: {
          objectKey: 'forge/task/a',
          typeId: 'forge/task',
        },
        data: {
          name: 'Build input',
          taskState: 4,
        },
      },
      {
        entity: {
          objectKey: 'forge/task/b',
          typeId: 'forge/task',
        },
        data: {
          name: 'Ship output',
          taskState: 2,
        },
      },
    ],
    loading: false,
  }),
}))

vi.mock('./useForgeTaskDependencyGraph.js', () => ({
  useForgeTaskDependencyGraph: () => mockDependencyGraph,
}))

vi.mock('../space/useVisibleObjectWizardTypeSet.js', () => ({
  useVisibleObjectWizardTypeSet: () => mockVisibleWizardTypeSet,
}))

describe('ForgeJobViewer', () => {
  const mockApplyWorldOp = vi
    .fn<
      (
        opTypeId: string,
        opData: Uint8Array,
        auth: string,
      ) => Promise<{ seqno: bigint; sysErr: boolean }>
    >()
    .mockResolvedValue({ seqno: 1n, sysErr: false })
  const mockSpaceWorld = {
    applyWorldOp: mockApplyWorldOp,
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
        <ForgeJobViewer
          objectInfo={{
            info: {
              case: 'worldObjectInfo',
              value: {
                objectKey: 'forge/job/main',
                objectType: 'forge/job',
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
    mockDependencyGraph.error = null
    mockVisibleWizardTypeSet.clear()
    mockVisibleWizardTypeSet.add('forge/task')
    vi.clearAllMocks()
  })

  it('opens the task wizard with the current job preselected', async () => {
    const user = userEvent.setup()
    renderViewer()

    await user.click(screen.getByRole('button', { name: 'Tasks' }))
    await user.click(screen.getByRole('button', { name: /add task/i }))

    expect(mockSpaceWorld.applyWorldOp).toHaveBeenCalledTimes(1)
    const [opTypeId, opData] = mockApplyWorldOp.mock.calls[0]
    expect(opTypeId).toBe(CREATE_WIZARD_OBJECT_OP_ID)

    const decoded = CreateWizardObjectOp.fromBinary(opData)
    expect(decoded.objectKey).toBe('wizard/task-1')
    expect(decoded.wizardTypeId).toBe('wizard/forge/task')
    expect(decoded.targetTypeId).toBe('forge/task')
    expect(decoded.initialStep).toBe(1)

    const initialConfigData = decoded.initialConfigData
    if (!initialConfigData) {
      throw new Error('expected initial task config data')
    }
    const config = ForgeTaskCreateOp.fromBinary(initialConfigData)
    expect(config.jobKey).toBe('forge/job/main')
    expect(mockNavigateToObjects).toHaveBeenCalledWith([decoded.objectKey])
  })

  it('renders task progress and the dependency graph view', async () => {
    const user = userEvent.setup()
    renderViewer()

    expect(screen.getAllByText('1/2').length).toBeGreaterThan(0)
    expect(screen.getAllByText('50% complete').length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: 'Tasks' }))
    expect(screen.getAllByText('Build input').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Ship output').length).toBeGreaterThan(0)

    const [dagButton] = screen.getAllByRole('button', { name: /^dag$/i })
    if (!dagButton) {
      throw new Error('expected dag toggle')
    }
    await user.click(dagButton)

    expect(screen.getAllByText('subtask').length).toBeGreaterThan(0)
  })

  it('keeps a published dependency truncation alert visible across tabs', async () => {
    const user = userEvent.setup()
    mockDependencyGraph.error = new Error(
      'Forge dependency graph exceeds the 50-edge limit for forge/task/a.',
    )

    renderViewer()

    expect(screen.getByRole('alert').textContent).toContain(
      'exceeds the 50-edge limit',
    )
    expect(screen.getByText('1/2')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Tasks' }))
    expect(screen.getByText('Build input')).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain(
      'exceeds the 50-edge limit',
    )
  })

  it('hides the add-task affordance when forge tasks are experimental', async () => {
    mockVisibleWizardTypeSet.clear()

    renderViewer()
    await userEvent.click(screen.getByRole('button', { name: 'Tasks' }))

    expect(screen.queryByRole('button', { name: /add task/i })).toBeNull()
  })
})
