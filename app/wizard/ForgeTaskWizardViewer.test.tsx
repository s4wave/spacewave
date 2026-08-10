import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'

import { ForgeTaskWizardViewer } from './ForgeTaskWizardViewer.js'

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

let currentStep = 1

function JobEditorStub({
  onValueChange,
}: {
  onValueChange?: (value: ForgeTaskCreateOp) => void
}) {
  return (
    <button
      type="button"
      onClick={() =>
        onValueChange?.(ForgeTaskCreateOp.create({ jobKey: 'forge/job/retry' }))
      }
    >
      Select job
    </button>
  )
}

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
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWTaskPeer',
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
      element: <JobEditorStub />,
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
    h.updateState.mockReset().mockResolvedValue(undefined)
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

    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    const [opTypeId, opData, sender] = h.applyWorldOp.mock.calls[0] as [
      string,
      Uint8Array,
      string,
    ]
    expect(opTypeId).toBe('spacewave/forge/task/create')
    expect(sender).toBe('12D3KooWTaskPeer')

    const decoded = ForgeTaskCreateOp.fromBinary(opData)
    expect(decoded.taskKey).toBe('compile-task-1')
    expect(decoded.name).toBe('Compile Task')
    expect(decoded.jobKey).toBe('forge/job/main')
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/forge/task/test')
    expect(h.navigateToObjects).toHaveBeenCalledWith([decoded.taskKey])
    expect(h.toastSuccess).toHaveBeenCalledWith('Created task Compile Task')
  })
  it('retries a failed job-selection transition', async () => {
    currentStep = 0
    h.updateState
      .mockRejectedValueOnce(new Error('temporary persistence failure'))
      .mockResolvedValueOnce(undefined)
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

    await user.click(screen.getByRole('button', { name: 'Select job' }))
    expect((await screen.findByRole('alert')).textContent).toContain(
      'temporary persistence failure',
    )

    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(h.updateState).toHaveBeenCalledTimes(2)
    expect(h.updateState).toHaveBeenLastCalledWith(
      expect.objectContaining({ step: 1 }),
    )
  })
})
