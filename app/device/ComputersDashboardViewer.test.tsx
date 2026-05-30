import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { CreateWizardObjectOp } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { CREATE_WIZARD_OBJECT_OP_ID } from '@s4wave/sdk/world/wizard/create-wizard.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { SshHostTypeID } from '@s4wave/sdk/sshhost/sshhost.js'

import {
  AddDeviceDefaultName,
  AddDeviceWizardTargetKeyPrefix,
  AddDeviceWizardTypeID,
} from './add-device-wizard.js'

type ApplyWorldOp = (
  opTypeId: string,
  opData: Uint8Array,
  opSender: string,
) => Promise<{ seqno: bigint; sysErr: boolean }>

const h = vi.hoisted<{
  applyWorldOp: ReturnType<typeof vi.fn<ApplyWorldOp>>
  navigateToObjects: ReturnType<typeof vi.fn<(objectKeys: string[]) => void>>
  visibleWizardTypes: Set<string>
  objects: Array<{ objectKey: string; objectType: string }>
}>(() => ({
  applyWorldOp: vi
    .fn<ApplyWorldOp>()
    .mockResolvedValue({ seqno: 1n, sysErr: false }),
  navigateToObjects: vi.fn<(objectKeys: string[]) => void>(),
  visibleWizardTypes: new Set<string>(['spacewave/device']),
  objects: [
    { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
    { objectKey: 'hosts/prod', objectType: 'spacewave/ssh-host' },
  ],
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({
      spaceState: { worldContents: { objects: h.objects } },
      spaceWorld: { applyWorldOp: h.applyWorldOp },
      navigateToObjects: h.navigateToObjects,
    }),
  },
}))

vi.mock('../space/useVisibleObjectWizardTypeSet.js', () => ({
  useVisibleObjectWizardTypeSet: () => h.visibleWizardTypes,
}))

import { ComputersDashboardViewer } from './ComputersDashboardViewer.js'

describe('ComputersDashboardViewer', () => {
  beforeEach(() => {
    h.visibleWizardTypes = new Set(['spacewave/device'])
    h.objects = [
      { objectKey: 'devices/build-host', objectType: 'spacewave/device' },
      { objectKey: 'hosts/prod', objectType: SshHostTypeID },
    ]
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('lists Device objects and future host entries without hiding empty sections', () => {
    render(
      <ComputersDashboardViewer
        objectInfo={{}}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.getByText('Computers')).toBeTruthy()
    expect(screen.getByText('devices/build-host')).toBeTruthy()
    expect(screen.getByText('hosts/prod')).toBeTruthy()
    expect(screen.getAllByText('1')).toHaveLength(2)
  })

  it('launches the Add Device wizard through the persistent wizard op', async () => {
    render(
      <ComputersDashboardViewer
        objectInfo={{}}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: /add device/i }))

    expect(h.applyWorldOp).toHaveBeenCalledWith(
      CREATE_WIZARD_OBJECT_OP_ID,
      expect.any(Uint8Array),
      '',
    )
    const opData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(opData instanceof Uint8Array)) {
      throw new Error('expected wizard op bytes')
    }
    const op = CreateWizardObjectOp.fromBinary(opData)
    expect(op.objectKey).toBe('wizard/device-1')
    expect(op.wizardTypeId).toBe(AddDeviceWizardTypeID)
    expect(op.targetTypeId).toBe(DeviceTypeID)
    expect(op.targetKeyPrefix).toBe(AddDeviceWizardTargetKeyPrefix)
    expect(op.name).toBe(AddDeviceDefaultName)
    await waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith(['wizard/device-1']),
    )
  })

  it('opens the Device row so DeviceViewer owns terminal capability actions', () => {
    render(
      <ComputersDashboardViewer
        objectInfo={{}}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    fireEvent.click(
      screen.getByRole('button', { name: /devices\/build-host/i }),
    )

    expect(h.navigateToObjects).toHaveBeenCalledWith(['devices/build-host'])
  })
})
