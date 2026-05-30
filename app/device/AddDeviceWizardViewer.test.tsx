import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'
import { SET_SPACE_SETTINGS_OP_ID } from '@s4wave/core/space/world/ops/set-space-settings.js'
import {
  SpaceLinkCallback,
  SpaceLinkCallbackStatus,
  SpaceLinkCompletionMode,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import { ComputersDashboardTypeID } from './computers.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  deleteObject: vi.fn().mockResolvedValue({ deleted: true }),
  navigateToObjects: vi.fn(),
  updateState: vi.fn().mockResolvedValue(undefined),
  persistDraftState: vi.fn().mockResolvedValue(undefined),
  handleConfigDataChange: vi.fn(),
  setCreating: vi.fn(),
  previewSpaceLink: vi.fn(),
  approveSpaceLink: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  currentStep: 1,
  configData: undefined as Uint8Array | undefined,
  spaceSettingsIndexPath: 'wizard/device-setup',
  worldObjects: [
    { objectKey: 'computers', objectType: 'spacewave/computers' },
    { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
  ],
}))

vi.mock('../wizard/useWizardState.js', () => ({
  useWizardState: () => ({
    objectKey: 'wizard/device-setup',
    state: {
      step: h.currentStep,
      targetTypeId: 'spacewave/device',
      targetKeyPrefix: 'devices/',
      name: 'Build Host',
    },
    localName: 'Build Host',
    creating: false,
    setCreating: h.setCreating,
    sessionPeerId: '12D3KooWSession',
    spaceWorld: {
      applyWorldOp: h.applyWorldOp,
      deleteObject: h.deleteObject,
    },
    spaceSettings: {
      indexPath: h.spaceSettingsIndexPath,
      pluginIds: ['spacewave-web'],
    },
    existingObjectKeys: h.worldObjects.map((obj) => obj.objectKey),
    navigateToObjects: h.navigateToObjects,
    wizardResource: { value: { updateState: h.updateState } },
    configEditor: { element: null, value: undefined },
    configData: h.configData,
    persistDraftState: h.persistDraftState,
    handleConfigDataChange: h.handleConfigDataChange,
    handleUpdateName: vi.fn(),
    handleBack: vi.fn(),
    handleCancel: vi.fn(),
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: (resource: { value?: unknown }) => resource.value,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: {
    useContext: () => ({
      value: {
        spacewave: {
          previewSpaceLink: h.previewSpaceLink,
          approveSpaceLink: h.approveSpaceLink,
        },
      },
    }),
  },
  SharedObjectContext: {
    useContext: () => ({ value: { meta: { sharedObjectId: 'space-1' } } }),
  },
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({
      spaceState: { worldContents: { objects: h.worldObjects } },
    }),
  },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: h.toastSuccess,
    error: h.toastError,
  },
}))

import { AddDeviceWizardViewer } from './AddDeviceWizardViewer.js'

describe('AddDeviceWizardViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.currentStep = 1
    h.configData = undefined
    h.spaceSettingsIndexPath = 'wizard/device-setup'
    h.worldObjects = [
      { objectKey: 'computers', objectType: ComputersDashboardTypeID },
      { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
    ]
  })

  it('renders CLI and container setup copy distinct from account device pairing', () => {
    renderViewer()

    expect(screen.getByText('Add Device')).toBeTruthy()
    expect(screen.queryByText('Link My Device')).toBeNull()
    expect(screen.getAllByText(/spacewave device setup/)).toHaveLength(2)
    expect(screen.getAllByText(/--target-hint space-1/)).toHaveLength(2)
  })

  it('approves a CLI SpaceLink ticket and persists the completion command', async () => {
    const ticket = bytesToBase64(new Uint8Array([1, 2, 3]))
    h.previewSpaceLink.mockResolvedValue({
      label: 'Build Host',
      agentPeerId: new Uint8Array([4, 5]),
      targetHint: new TextEncoder().encode('space-1'),
      completionMode: SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI,
    })
    h.approveSpaceLink.mockResolvedValue({
      completionMode: SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI,
      completion: {
        status: SpaceLinkCallbackStatus.SpaceLinkCallbackStatus_OK,
        nonce: new Uint8Array([9]),
      },
    })
    h.configData = new TextEncoder().encode(JSON.stringify({ ticket }))
    renderViewer()

    expect(
      screen.getByPlaceholderText(/paste the base64 ticket/i),
    ).toHaveProperty('value', ticket)
    fireEvent.click(screen.getByRole('button', { name: /approve/i }))

    await waitFor(() => expect(h.updateState).toHaveBeenCalled())
    expect(h.previewSpaceLink).toHaveBeenCalledWith({
      ticket: new Uint8Array([1, 2, 3]),
    })
    expect(h.approveSpaceLink).toHaveBeenCalledWith({
      ticket: new Uint8Array([1, 2, 3]),
      resourceId: new TextEncoder().encode('space-1'),
    })
    const update = h.updateState.mock.calls.at(-1)?.[0] as
      | { step?: number; configData?: Uint8Array }
      | undefined
    if (!update?.configData) {
      throw new Error('expected wizard state update')
    }
    expect(update.step).toBe(2)
    const config = JSON.parse(new TextDecoder().decode(update.configData)) as {
      ticket?: string
      completion?: string
    }
    expect(config.ticket).toBe(ticket)
    if (!config.completion) {
      throw new Error('expected completion')
    }
    const completion = SpaceLinkCallback.fromBinary(
      base64ToBytes(config.completion),
    )
    expect(completion.status).toBe(
      SpaceLinkCallbackStatus.SpaceLinkCallbackStatus_OK,
    )
  })

  it('promotes Computers after completion and opens the created Device object', async () => {
    h.currentStep = 2
    h.configData = new TextEncoder().encode(
      JSON.stringify({ completion: 'completion' }),
    )
    renderViewer()

    fireEvent.click(screen.getByRole('button', { name: /open device/i }))

    await waitFor(() => expect(h.deleteObject).toHaveBeenCalled())
    expect(h.applyWorldOp).toHaveBeenCalledWith(
      SET_SPACE_SETTINGS_OP_ID,
      expect.any(Uint8Array),
      '12D3KooWSession',
    )
    const settingsOpData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(settingsOpData instanceof Uint8Array)) {
      throw new Error('expected settings op bytes')
    }
    const settingsOp = SetSpaceSettingsOp.fromBinary(settingsOpData)
    expect(settingsOp.settings?.indexPath).toBe('computers')
    expect(settingsOp.settings?.pluginIds).toEqual(['spacewave-web'])
    expect(h.deleteObject).toHaveBeenCalledWith('wizard/device-setup')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['devices/build-host'])
  })
})

function renderViewer() {
  render(
    <AddDeviceWizardViewer
      objectInfo={{}}
      worldState={{
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    />,
  )
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value)
  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}
