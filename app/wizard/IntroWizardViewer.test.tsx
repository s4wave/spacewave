import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import { IntroWizardConfig } from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { IntroWizardViewer } from './IntroWizardViewer.js'
import { driveIntroConfig } from './intro.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  hasState: true,
  navigateToObjects: vi.fn(),
  spaceSettingsIndexPath: 'wizard/welcome-1',
  setCreating: vi.fn(),
  handleCancel: vi.fn(),
  toastError: vi.fn(),
}))

vi.mock('./useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/welcome-1',
    state: h.hasState
      ? {
          step: 0,
          targetTypeId: 'unixfs/fs-node',
          targetKeyPrefix: 'files',
          name: 'Welcome',
          configData: IntroWizardConfig.toBinary(driveIntroConfig()),
        }
      : undefined,
    localName: 'Welcome',
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWIntroPeer',
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
    persistDraftState: vi.fn().mockResolvedValue(undefined),
    handleConfigDataChange: vi.fn(),
    handleUpdateName: vi.fn(),
    handleBack: vi.fn(),
    handleCancel: h.handleCancel,
  }),
}))

vi.mock('@s4wave/web/object/ObjectViewer.js', () => ({
  ObjectViewer: () => <div data-testid="object-viewer" />,
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    error: h.toastError,
  },
}))

const viewerProps = {
  objectInfo: {},
  worldState: {
    value: null,
    loading: false,
    error: null,
    retry: vi.fn(),
  },
} as unknown as ObjectViewerComponentProps

describe('IntroWizardViewer', () => {
  beforeEach(() => {
    h.hasState = true
    h.spaceSettingsIndexPath = 'wizard/welcome-1'
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the intro content around the contained object viewer', () => {
    render(<IntroWizardViewer {...viewerProps} />)

    expect(screen.getByTestId('object-viewer')).toBeTruthy()
    expect(screen.getByText('Welcome to your Drive')).toBeTruthy()
    expect(screen.getByText('Add files')).toBeTruthy()
    expect(screen.getByText('Upload progress')).toBeTruthy()
  })

  it('sets the index to the introduced object, deletes the wizard, and navigates', async () => {
    const user = userEvent.setup()

    render(<IntroWizardViewer {...viewerProps} />)

    await user.click(
      screen.getByRole('button', { name: /got it, start exploring/i }),
    )

    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
    expect(h.applyWorldOp.mock.calls[0]?.[0]).toBe(SET_SPACE_SETTINGS_OP_ID)
    const opData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(opData instanceof Uint8Array)) {
      throw new Error('expected settings op bytes')
    }
    const settingsOp = SetSpaceSettingsOp.fromBinary(opData)
    expect(settingsOp.settings?.indexPath).toBe('files')
    expect(settingsOp.settings?.pluginIds).toEqual(['spacewave-web'])
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/welcome-1')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['files'])
  })

  it('does not replace a stale Space index before cleanup', async () => {
    const user = userEvent.setup()
    h.spaceSettingsIndexPath = 'files'

    render(<IntroWizardViewer {...viewerProps} />)

    await user.click(
      screen.getByRole('button', { name: /got it, start exploring/i }),
    )

    expect(h.applyWorldOp).not.toHaveBeenCalled()
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/welcome-1')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['files'])
  })

  it('renders loading while wizard state is unavailable', () => {
    h.hasState = false

    render(<IntroWizardViewer {...viewerProps} />)

    expect(screen.getByText('Loading')).toBeTruthy()
  })
})
