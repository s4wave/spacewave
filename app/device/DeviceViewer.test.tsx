import React, { type ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'

import {
  DeviceAvailabilityState,
  DeviceCapabilityGrantState,
  DeviceCapabilityLocalState,
  DeviceCapabilityState,
  DeviceCheckoutRootAccess,
  DeviceLiveness,
  DeviceSetupState,
  DeviceUpdateState,
  type Device,
} from '@s4wave/sdk/device/device.pb.js'
import {
  CreateTerminalOp,
  TerminalTargetKind,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { CreateDisplayOp } from '@s4wave/sdk/display/display.pb.js'
import { CreateDisplayOpTypeID } from '@s4wave/sdk/display/display.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import { keysWorld } from '@s4wave/web/test/world-query.js'

const h = vi.hoisted(() => ({
  streamRetry: vi.fn(),
  accessCheckoutRoot: vi.fn(),
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  navigateToObjects: vi.fn(),
  objects: [{ objectKey: 'terminal/build-host-terminal-1' }],
}))

let availability = DeviceAvailabilityState.OFFLINE
let currentState: Device | undefined = {
  peerId: '12D3KooWDevice',
  label: 'Build Host',
  platform: { os: 'linux', arch: 'arm64' },
  daemonVersion: 'test',
  setupState: DeviceSetupState.DEVICE_SESSION_READY,
  updateState: DeviceUpdateState.READY,
  lastStatus: {
    liveness: DeviceLiveness.DEGRADED,
    message: 'network limited',
  },
  capabilities: [
    {
      id: 'filesystem',
      kind: 'filesystem',
      label: 'Files',
      state: DeviceCapabilityState.AVAILABLE,
    },
    {
      id: 'terminal',
      kind: 'remote-shell',
      label: 'Terminal',
      state: DeviceCapabilityState.DISABLED,
      detail: 'disabled by local policy',
    },
    {
      id: 'forge-worker',
      kind: 'forge-worker',
      label: 'Forge Worker',
      state: DeviceCapabilityState.GRANT_BLOCKED,
      detail: 'blocked by Space grant',
    },
    {
      id: 'remote-shell-session',
      kind: 'remote-shell',
      label: 'Shell Session',
      state: DeviceCapabilityState.ACTIVE,
    },
  ],
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => {
  const value = {
    watchDeviceAvailability: vi.fn(),
    accessCheckoutRoot: h.accessCheckoutRoot,
  }
  return {
    useAccessTypedHandle: () => ({
      value,
      loading: false,
      error: null,
      retry: vi.fn(),
    }),
  }
})

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: currentState
      ? { device: currentState, state: availability }
      : undefined,
    loading: false,
    error: null,
    retry: h.streamRetry,
  }),
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({
      spaceWorld: {
        ...keysWorld(() => h.objects.map((obj) => obj.objectKey)),
        applyWorldOp: h.applyWorldOp,
      },
      navigateToObjects: h.navigateToObjects,
    }),
  },
}))

vi.mock('@s4wave/app/unixfs/UnixFSBrowser.js', () => ({
  UnixFSBrowser: ({
    editing,
    actions,
  }: {
    editing: boolean
    actions: ReactNode
  }) => (
    <>
      {actions}
      {editing && <button type="button">New folder</button>}
    </>
  ),
}))

import { DeviceViewer } from './DeviceViewer.js'

describe('DeviceViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    currentState = undefined
    availability = DeviceAvailabilityState.OFFLINE
    h.objects = [{ objectKey: 'terminal/build-host-terminal-1' }]
  })

  it('renders identity, status, and capability inventory', () => {
    render(
      <DeviceViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'devices/build-host',
              objectType: 'spacewave/device',
            },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.getByText('Build Host')).toBeTruthy()
    expect(screen.getByText('Offline')).toBeTruthy()
    expect(screen.getAllByText('Ready')).toHaveLength(2)
    expect(screen.getByText('network limited')).toBeTruthy()
    expect(screen.getByTitle('12D3KooWDevice').textContent).toBe(
      '12D3KooWDevice',
    )
    expect(screen.getByRole('button', { name: 'Copy Peer ID' })).toBeTruthy()
    expect(screen.getByText('Files')).toBeTruthy()
    expect(screen.getByText('Available')).toBeTruthy()
    expect(screen.getByText('Terminal')).toBeTruthy()
    expect(screen.getByText('Disabled')).toBeTruthy()
    expect(screen.getByText('blocked by Space grant')).toBeTruthy()
    expect(screen.getByText('Grant blocked')).toBeTruthy()
    expect(screen.getByText('Shell Session')).toBeTruthy()
    expect(screen.getByText('Active')).toBeTruthy()
    expect(screen.getByRole('button', { name: /open terminal/i })).toBeTruthy()
  })

  it('opens the selected checkout root and waits for its Device while offline', () => {
    // Publish one readable folder on an offline Device.
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'project',
          kind: 'filesystem',
          label: 'Project files',
          state: DeviceCapabilityState.ACTIVE,
          link: { protocolId: 'checkout-root/project' },
          checkoutRoot: {
            name: 'Project files',
            access: DeviceCheckoutRootAccess.READ_ONLY,
            readAvailable: true,
          },
        },
      ],
    }
    renderDevice()

    // Browse waits for the Device and offers a way back to its capabilities.
    fireEvent.click(
      screen.getByRole('button', { name: 'Browse Project files' }),
    )
    expect(
      screen.getByText('Device offline. The folder opens when it returns.'),
    ).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Enable editing' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Back to Device' }))
    expect(
      screen.getByRole('button', { name: 'Browse Project files' }),
    ).toBeTruthy()
  })

  it('keeps editing confirmation when the Device reconnects', async () => {
    // The handle opens with the capability's access while edits wait for consent.
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      capabilities: [
        {
          id: 'project',
          kind: 'filesystem',
          label: 'Project files',
          state: DeviceCapabilityState.ACTIVE,
          link: { protocolId: 'checkout-root/project' },
          policy: {
            localState: DeviceCapabilityLocalState.ENABLED,
            grantState: DeviceCapabilityGrantState.ALLOWED,
          },
          checkoutRoot: {
            name: 'Project files',
            access: DeviceCheckoutRootAccess.READ_WRITE,
            readAvailable: true,
            writeAvailable: true,
          },
        },
      ],
    }
    const disposeFirstRoot = vi.fn()
    h.accessCheckoutRoot
      .mockResolvedValueOnce({ handle: { [Symbol.dispose]: disposeFirstRoot } })
      .mockResolvedValueOnce({ handle: { [Symbol.dispose]: vi.fn() } })
    availability = DeviceAvailabilityState.ONLINE
    const { refresh } = renderDevice()
    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: 'Browse Project files' }),
      )
    })
    expect(h.accessCheckoutRoot).toHaveBeenCalledWith({
      name: 'Project files',
      write: true,
      abortSignal: expect.any(AbortSignal),
    })
    expect(screen.queryByRole('button', { name: 'New folder' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Enable editing' }))
    expect(screen.getByRole('button', { name: 'New folder' })).toBeTruthy()

    // A Device watch transition closes the old handle and reopens the next one.
    availability = DeviceAvailabilityState.OFFLINE
    refresh()
    expect(screen.queryByRole('button', { name: 'New folder' })).toBeNull()
    expect(disposeFirstRoot).toHaveBeenCalledOnce()
    availability = DeviceAvailabilityState.ONLINE
    await act(async () => refresh())
    expect(h.accessCheckoutRoot).toHaveBeenCalledTimes(2)
    expect(screen.getByRole('button', { name: 'New folder' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Enable editing' })).toBeNull()
  })

  it('creates a Terminal object from effective terminal capability state', async () => {
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'terminal',
          kind: 'remote-shell',
          label: 'Terminal',
          state: DeviceCapabilityState.AVAILABLE,
        },
      ],
    }

    render(
      <DeviceViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'devices/build-host',
              objectType: 'spacewave/device',
            },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    screen.getByRole('button', { name: /open terminal/i }).click()

    await vi.waitFor(() =>
      expect(h.applyWorldOp).toHaveBeenCalledWith(
        CREATE_TERMINAL_OP_ID,
        expect.any(Uint8Array),
        '',
      ),
    )
    const opData: unknown = h.applyWorldOp.mock.calls[0]?.[1]
    if (!(opData instanceof Uint8Array)) {
      throw new Error('expected terminal op bytes')
    }
    const op = CreateTerminalOp.fromBinary(opData)
    expect(op.objectKey).toBe('build-host-terminal-1')
    expect(op.name).toBe('Build Host Terminal')
    expect(op.deviceObjectKey).toBe('devices/build-host')
    expect(op.devicePeerId).toBe('12D3KooWDevice')
    expect(op.targetKind).toBe(TerminalTargetKind.DEVICE)
    expect(op.cols).toBe(80)
    expect(op.rows).toBe(24)
    expect(h.navigateToObjects).toHaveBeenCalledWith(['build-host-terminal-1'])
  })

  it('opens the terminal from its capability row and only selects others', async () => {
    // The device can open a terminal and also lists files.
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'filesystem',
          kind: 'filesystem',
          label: 'Files',
          state: DeviceCapabilityState.AVAILABLE,
        },
        {
          id: 'terminal',
          kind: 'remote-shell',
          label: 'Terminal',
          state: DeviceCapabilityState.AVAILABLE,
        },
      ],
    }
    renderDevice()

    // Opening the Files row selects it and opens nothing.
    const files = screen.getByText('Files').closest('[role="row"]')!
    fireEvent.click(files)
    fireEvent.doubleClick(files)
    expect(files.getAttribute('aria-selected')).toBe('true')

    // Let an open's awaited op build settle before checking nothing ran.
    await act(() => new Promise((resolve) => setTimeout(resolve, 0)))
    expect(h.applyWorldOp).not.toHaveBeenCalled()

    // Opening the Terminal row runs Open Terminal.
    const terminal = screen
      .getAllByText('Terminal')
      .map((el) => el.closest('[role="row"]'))
      .find((row) => row != null)!
    fireEvent.click(terminal)
    fireEvent.doubleClick(terminal)
    await vi.waitFor(() =>
      expect(h.navigateToObjects).toHaveBeenCalledWith([
        'build-host-terminal-1',
      ]),
    )
    expect(h.applyWorldOp).toHaveBeenCalledTimes(1)
  })

  it('creates a Display from its Device capability row', async () => {
    // A Device can publish a live display next to a disabled display row.
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      capabilities: [
        {
          id: 'flowgraph-node/flowgraphs/desk/display',
          kind: 'display',
          label: 'Desktop',
          state: DeviceCapabilityState.ACTIVE,
        },
        {
          id: 'disabled-display',
          kind: 'display',
          label: 'Disabled display',
          state: DeviceCapabilityState.REJECTED,
        },
      ],
    }
    renderDevice()
    expect(
      screen.queryByRole('button', { name: 'Open Disabled display' }),
    ).toBeNull()

    // Open the capability by its row and await navigation from the create op.
    const opened = Promise.withResolvers<string[]>()
    h.navigateToObjects.mockImplementationOnce(opened.resolve)
    const row = screen.getByText('Desktop').closest('[role="row"]')!
    fireEvent.click(row)
    fireEvent.doubleClick(row)
    const [objectKey] = await opened.promise

    // The new object follows the Device key and capability across re-enrollment.
    const [typename, data] = h.applyWorldOp.mock.calls[0]!
    expect(typename).toBe(CreateDisplayOpTypeID)
    expect(CreateDisplayOp.fromBinary(data)).toMatchObject({
      objectKey,
      name: 'Desktop',
      deviceObjectKey: 'devices/build-host',
      capabilityId: 'flowgraph-node/flowgraphs/desk/display',
    })
    expect(objectKey).toBe('desktop-1')
  })

  it('does not expose Terminal action for disabled terminal capability', () => {
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      capabilities: [
        {
          id: 'terminal',
          kind: 'remote-shell',
          label: 'Terminal',
          state: DeviceCapabilityState.DISABLED,
        },
      ],
    }

    render(
      <DeviceViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'devices/build-host',
              objectType: 'spacewave/device',
            },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.queryByRole('button', { name: /open terminal/i })).toBeNull()
  })

  it('offers Retry when the stream ends without a device', () => {
    render(
      <DeviceViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'devices/build-host',
              objectType: 'spacewave/device',
            },
          },
        }}
        worldState={{
          value: null,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
      />,
    )

    expect(screen.getByText('The Device returned no details.')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(h.streamRetry).toHaveBeenCalledTimes(1)
  })
})

// renderDevice renders the viewer on the build host Device object.
function renderDevice() {
  const view = (
    <DeviceViewer
      objectInfo={{
        info: {
          case: 'worldObjectInfo',
          value: {
            objectKey: 'devices/build-host',
            objectType: 'spacewave/device',
          },
        },
      }}
      worldState={{
        value: null,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    />
  )
  const rendered = render(view)
  return {
    ...rendered,
    refresh: () => rendered.rerender(React.cloneElement(view)),
  }
}
