import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { ForgeJobWizardViewer } from './ForgeJobWizardViewer.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  navigateToObjects: vi.fn(),
  setCreating: vi.fn(),
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
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWJobPeer',
    spaceWorld: {
      applyWorldOp: h.applyWorldOp,
      deleteObject: h.deleteObject,
    },
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
  })

  it('advances from config step when cluster and tasks are configured', async () => {
    const user = userEvent.setup()
    render(
      <ForgeJobWizardViewer
        objectInfo={{} as never}
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
        objectInfo={{} as never}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /create/i }))

    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    const [opTypeId, opData, sender] = h.applyWorldOp.mock.calls[0] as [
      string,
      Uint8Array,
      string,
    ]
    expect(opTypeId).toBe('spacewave/forge/job/create')
    expect(sender).toBe('12D3KooWJobPeer')

    const decoded = ForgeJobCreateOp.fromBinary(opData)
    expect(decoded.jobKey).toBe('build-job-1')
    expect(decoded.clusterKey).toBe('forge/cluster/main')
    expect(decoded.taskDefs?.map((td) => td.name)).toEqual(['compile', 'test'])
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/forge/job/test')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.jobKey])
    expect(h.toastSuccess).toHaveBeenCalledWith('Created Build Job')
  })
})
