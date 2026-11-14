import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'

import { ForgeTaskWizardViewer } from './ForgeTaskWizardViewer.js'
import { wizardTestWorld, type WizardTestWorld } from './wizard-test-world.js'

const h = vi.hoisted(() => ({
  t: undefined as unknown as WizardTestWorld,
  navigateToObjects: vi.fn(),
  updateState: vi.fn(),
  handleUpdateName: vi.fn(),
  handleBack: vi.fn(),
  handleCancel: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}))

let currentStep = 1

vi.mock('./useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/forge/task/test',
    state: {
      step: currentStep,
      targetTypeId: 'forge/task',
      targetKeyPrefix: 'forge/task/',
      name: 'Compile Task',
    },
    localName: 'Compile Task',
    sessionPeerId: '12D3KooWTaskPeer',
    spaceSettings: { indexPath: 'wizard/forge/task/test' },
    spaceWorld: h.t.world,
    navigateToObjects: h.navigateToObjects,
    wizardResource: {
      value: {
        updateState: h.updateState,
      },
    },
    configEditor: {
      element: <div>Forge Task Config</div>,
      value: {
        jobKey: 'forge/job/main',
      },
    },
    persistDraftState: vi.fn().mockResolvedValue(undefined),
    handleConfigDataChange: vi.fn(),
    handleUpdateName: h.handleUpdateName,
    handleBack: h.handleBack,
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

describe('ForgeTaskWizardViewer', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  beforeEach(() => {
    currentStep = 1
    h.t = wizardTestWorld(
      ['wizard/forge/task/test'],
      'wizard/forge/task/test',
      keysWorld(() => []),
    )
  })

  it('finalizes a forge task linked to the selected job', async () => {
    const user = userEvent.setup()
    render(
      <ForgeTaskWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /create/i }))

    // One Finish applied the task create.
    await vi.waitFor(() => expect(h.navigateToObjects).toHaveBeenCalled())
    const opData = h.t.opData.get('spacewave/forge/task/create')
    const decoded = ForgeTaskCreateOp.fromBinary(opData ?? new Uint8Array())
    expect(decoded.taskKey).toBe('compile-task')
    expect(decoded.name).toBe('Compile Task')
    expect(decoded.jobKey).toBe('forge/job/main')

    // The task, the index move and the wizard delete committed together.
    expect(h.t.state).toEqual({
      objects: new Set(),
      indexPath: decoded.taskKey,
      creates: ['spacewave/forge/task/create'],
    })
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.taskKey])
    expect(h.toastSuccess).toHaveBeenCalledWith('Created task Compile Task')
  })
})
