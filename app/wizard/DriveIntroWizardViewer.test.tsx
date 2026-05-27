import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'

import { DriveIntroWizardViewer } from './DriveIntroWizardViewer.js'
import { DriveIntroTargetObjectKey } from './drive-intro.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  hasState: true,
  navigateToObjects: vi.fn(),
  spaceSettingsIndexPath: 'wizard/drive-intro-test',
  setCreating: vi.fn(),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  handleUpdateName: vi.fn(),
  handleBack: vi.fn(),
  handleCancel: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('./useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/drive-intro-test',
    state: h.hasState
      ? {
          step: 0,
          targetTypeId: 'unixfs/fs-node',
          targetKeyPrefix: DriveIntroTargetObjectKey,
          name: 'My Drive',
        }
      : undefined,
    localName: 'My Drive',
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWDrivePeer',
    spaceWorld: {
      applyWorldOp: h.applyWorldOp,
      deleteObject: h.deleteObject,
    },
    spaceSettings: {
      indexPath: h.spaceSettingsIndexPath,
      pluginIds: ['spacewave-web'],
    },
    existingObjectKeys: [],
    navigateToObjects: h.navigateToObjects,
    wizardResource: { value: {} },
    configEditor: { element: null, value: undefined },
    configData: undefined,
    persistDraftState: h.persistDraftState,
    handleConfigDataChange: vi.fn(),
    handleUpdateName: h.handleUpdateName,
    handleBack: h.handleBack,
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    error: h.toastError,
  },
}))

describe('DriveIntroWizardViewer', () => {
  beforeEach(() => {
    h.hasState = true
    h.spaceSettingsIndexPath = 'wizard/drive-intro-test'
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('replaces the Space index, deletes the intro, and opens raw files', async () => {
    const user = userEvent.setup()

    render(
      <DriveIntroWizardViewer
        objectInfo={{}}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /open files/i }))

    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    expect(h.applyWorldOp.mock.calls[0]?.[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const opData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(opData instanceof Uint8Array)) {
      throw new Error('expected settings op bytes')
    }
    const settingsOp = SetSpaceSettingsOp.fromBinary(opData)
    expect(settingsOp.settings?.indexPath).toBe(DriveIntroTargetObjectKey)
    expect(settingsOp.settings?.pluginIds).toEqual(['spacewave-web'])
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/drive-intro-test')
    expect(h.navigateToObjects).toHaveBeenCalledWith([
      DriveIntroTargetObjectKey,
    ])
  })

  it('does not replace a stale Space index before cleanup', async () => {
    const user = userEvent.setup()
    h.spaceSettingsIndexPath = 'files'

    render(
      <DriveIntroWizardViewer
        objectInfo={{}}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    await user.click(screen.getByRole('button', { name: /open files/i }))

    expect(h.applyWorldOp).not.toHaveBeenCalled()
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/drive-intro-test')
    expect(h.navigateToObjects).toHaveBeenCalledWith([
      DriveIntroTargetObjectKey,
    ])
  })

  it('renders loading while wizard state is unavailable', () => {
    h.hasState = false

    render(
      <DriveIntroWizardViewer
        objectInfo={{}}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.getByText('Loading Drive')).toBeTruthy()
  })
})
