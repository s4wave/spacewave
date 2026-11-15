import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'
import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { Task } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import type { EntityRow, KeyedSet } from '@s4wave/sdk/world/entity-set.js'
import { ForgeJobViewer } from './ForgeJobViewer.js'

const mockVisibleWizardTypeSet = new Set(['forge/task'])

vi.mock('@s4wave/web/forge/ForgeViewerShell.js', () => ({
  forgeBlockStatus: () => null,
  ForgeViewerShell: ({
    details,
    tabs,
  }: {
    details?: { facts: React.ReactNode }
    tabs?: Array<{ id: string; content: React.ReactNode }>
  }) => (
    <div data-testid="forge-viewer-shell">
      {details?.facts}
      {tabs?.map((tab) => (
        <div key={tab.id}>{tab.content}</div>
      ))}
    </div>
  ),
}))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlock: () => ({
    value: {
      timestamp: new Date('2026-04-17T00:00:00Z'),
      jobState: 1,
    },
    loading: false,
    error: null,
    retry: () => {},
  }),
}))

// tasks is the followed task set the viewer reads; a test replaces it and
// renders again to move the set through its states.
const tasks = vi.hoisted(() => ({
  current: undefined as unknown as Resource<KeyedSet<EntityRow<Partial<Task>>>>,
}))

vi.mock('@s4wave/web/hooks/useEntitySet.js', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useEntitySet: () => tasks.current,
}))

// taskSet is a read task set; keys lists members whose bodies are not read.
function taskSet(
  rows: Record<string, Partial<Task>>,
  { keys = Object.keys(rows), complete = true, seqno = 1n } = {},
): KeyedSet<EntityRow<Partial<Task>>> {
  return {
    seqno,
    keys,
    rows: new Map(Object.entries(rows).map(([key, value]) => [key, { value }])),
    complete,
  }
}

// loaded is a resource holding set.
function loaded(
  set: KeyedSet<EntityRow<Partial<Task>>>,
): Resource<KeyedSet<EntityRow<Partial<Task>>>> {
  return resourceOf({ value: set, loading: false, error: null, retry: vi.fn() })
}

const buildInput = { name: 'Build input', taskState: 4 }
const shipOutput = { name: 'Ship output', taskState: 2 }

vi.mock('./useForgeTaskDependencyGraph.js', () => ({
  useForgeTaskDependencyGraph: () => ({
    edges: [
      {
        from: 'forge/task/a',
        to: 'forge/task/b',
        kind: 'subtask',
      },
    ],
    loading: false,
  }),
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
    ...keysWorld([]),
    applyWorldOp: mockApplyWorldOp,
  }
  const mockNavigateToObjects = vi.fn()

  function renderViewer() {
    const view = render(viewer())
    return { ...view, update: () => view.rerender(viewer()) }
  }

  function viewer() {
    return (
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
      </SpaceContainerContext.Provider>
    )
  }

  beforeEach(() => {
    tasks.current = loaded(
      taskSet({ 'forge/task/a': buildInput, 'forge/task/b': shipOutput }),
    )
  })

  afterEach(() => {
    cleanup()
    mockVisibleWizardTypeSet.clear()
    mockVisibleWizardTypeSet.add('forge/task')
    vi.clearAllMocks()
  })

  it('opens the task wizard with the current job preselected', async () => {
    const user = userEvent.setup()
    renderViewer()

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
    expect(screen.getAllByText('Build input').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Ship output').length).toBeGreaterThan(0)

    const [dagButton] = screen.getAllByRole('button', { name: /^dag$/i })
    if (!dagButton) {
      throw new Error('expected dag toggle')
    }
    await user.click(dagButton)

    expect(screen.getAllByText('subtask').length).toBeGreaterThan(0)
  })

  it('hides the add-task affordance when forge tasks are experimental', () => {
    mockVisibleWizardTypeSet.clear()

    renderViewer()

    expect(screen.queryByRole('button', { name: /add task/i })).toBeNull()
  })

  it('keeps the rows and never lowers progress while a task is linked', () => {
    const view = renderViewer()
    expect(screen.getAllByText('50% complete').length).toBeGreaterThan(0)

    // A third task is linked and read in the same wake: no loading line,
    // the rows stay, and the count and its row arrive together.
    tasks.current = loaded(
      taskSet({
        'forge/task/a': buildInput,
        'forge/task/b': shipOutput,
        'forge/task/c': { name: 'Publish', taskState: 4 },
      }),
    )
    view.update()
    expect(screen.queryByText('Loading tasks...')).toBeNull()
    expect(screen.getAllByText('Build input').length).toBeGreaterThan(0)
    expect(screen.getAllByText('2/3').length).toBeGreaterThan(0)
    expect(screen.getAllByText('67% complete').length).toBeGreaterThan(0)

    // A removed task leaves its row and the count.
    tasks.current = loaded(
      taskSet({
        'forge/task/a': buildInput,
        'forge/task/c': { name: 'Publish', taskState: 4 },
      }),
    )
    view.update()
    expect(screen.queryByText('Ship output')).toBeNull()
    expect(screen.getAllByText('2/2').length).toBeGreaterThan(0)
  })

  it('shows the first pages of a large job as they arrive', () => {
    tasks.current = loaded(
      taskSet(
        { 'forge/task/a': buildInput },
        {
          keys: ['forge/task/a', 'forge/task/b', 'forge/task/c'],
          complete: false,
        },
      ),
    )
    renderViewer()
    expect(screen.getAllByText('Build input').length).toBeGreaterThan(0)
    expect(screen.getByText('Loading 2 more')).toBeTruthy()
    expect(screen.queryByText('No tasks in job')).toBeNull()
  })

  it('reports a failure before the first read, never an empty job', () => {
    tasks.current = resourceOf({
      value: null,
      loading: false,
      error: new Error('stream reset'),
      retry: vi.fn(),
    })
    renderViewer()
    expect(screen.queryByText('No tasks in job')).toBeNull()
    expect(screen.getAllByText(/Could not read the tasks/).length).toBe(1)
    expect(screen.getAllByRole('button', { name: /retry/i }).length).toBe(1)
  })

  it('keeps the rows under a failure after the first read', () => {
    tasks.current = resourceOf({
      value: null,
      last: taskSet({ 'forge/task/a': buildInput }),
      loading: false,
      error: new Error('stream reset'),
      retry: vi.fn(),
    })
    renderViewer()
    expect(screen.getAllByText('Build input').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Could not read the tasks/).length).toBe(1)
  })
})
