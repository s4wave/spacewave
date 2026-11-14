import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'

import { ForgeJobWizardViewer } from './ForgeJobWizardViewer.js'
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

let currentStep = 0

vi.mock('./useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/forge/job/test',
    state: {
      step: currentStep,
      targetTypeId: 'forge/job',
      targetKeyPrefix: 'forge/job/',
      name: 'Build Job',
    },
    localName: 'Build Job',
    sessionPeerId: '12D3KooWJobPeer',
    spaceSettings: { indexPath: 'wizard/forge/job/test' },
    spaceWorld: h.t.world,
    navigateToObjects: h.navigateToObjects,
    wizardResource: {
      value: {
        updateState: h.updateState,
      },
    },
    configEditor: {
      element: <div>Forge Job Config</div>,
      value: {
        clusterKey: 'forge/cluster/main',
        taskDefs: [{ name: 'compile' }, { name: '' }, { name: 'test' }],
      },
    },
    persistDraftState: vi.fn().mockResolvedValue(undefined),
    handleConfigDataChange: vi.fn(),
    handleUpdateName: h.handleUpdateName,
    handleBack: h.handleBack,
    cancel: { status: 'idle' },
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

describe('ForgeJobWizardViewer', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  beforeEach(() => {
    currentStep = 0
    h.t = wizardTestWorld(
      ['wizard/forge/job/test'],
      'wizard/forge/job/test',
      keysWorld(() => []),
    )
  })

  it('advances from config step when cluster and tasks are configured', async () => {
    const user = userEvent.setup()
    render(
      <ForgeJobWizardViewer
        objectInfo={{}}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /^next$/i }))

    expect(h.updateState).toHaveBeenCalledWith({ step: 1 })
  })

  it('finalizes a forge job under the selected cluster', async () => {
    const user = userEvent.setup()
    currentStep = 1
    render(
      <ForgeJobWizardViewer
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

    // One Finish applied the job create.
    await vi.waitFor(() => expect(h.navigateToObjects).toHaveBeenCalled())
    const opData = h.t.opData.get('spacewave/forge/job/create')
    const decoded = ForgeJobCreateOp.fromBinary(opData ?? new Uint8Array())
    expect(decoded.jobKey).toBe('build-job')
    expect(decoded.clusterKey).toBe('forge/cluster/main')
    expect(decoded.taskDefs?.map((td) => td.name)).toEqual(['compile', 'test'])

    // The job, the index move and the wizard delete committed together.
    expect(h.t.state).toEqual({
      objects: new Set([decoded.jobKey]),
      indexPath: decoded.jobKey,
      creates: ['spacewave/forge/job/create'],
    })
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.jobKey])
    expect(h.toastSuccess).toHaveBeenCalledWith('Created Build Job')
  })
})
