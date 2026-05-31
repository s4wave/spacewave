import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import {
  DeviceCapabilityState,
  DeviceLiveness,
  DeviceSetupState,
  DeviceUpdateState,
  type Device,
} from '@s4wave/sdk/device/device.pb.js'
import {
  CreateTerminalOp,
  TerminalTargetKind,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  navigateToObjects: vi.fn(),
  objects: [{ objectKey: 'terminal/build-host-terminal-1' }],
}))

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
      kind: 'terminal',
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
      kind: 'terminal',
      label: 'Shell Session',
      state: DeviceCapabilityState.ACTIVE,
    },
  ],
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      watchDeviceState: vi.fn(),
    },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: currentState,
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
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

import { DeviceViewer } from './DeviceViewer.js'

describe('DeviceViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    currentState = undefined
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
    expect(screen.getByText('Degraded')).toBeTruthy()
    expect(screen.getAllByText('Ready')).toHaveLength(2)
    expect(screen.getByText('network limited')).toBeTruthy()
    expect(screen.getByText('12D3KooWDevice')).toBeTruthy()
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

  it('creates a Terminal object from effective terminal capability state', async () => {
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      setupState: DeviceSetupState.DEVICE_SESSION_READY,
      capabilities: [
        {
          id: 'terminal',
          kind: 'terminal',
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

  it('does not expose Terminal action for disabled terminal capability', () => {
    currentState = {
      peerId: '12D3KooWDevice',
      label: 'Build Host',
      capabilities: [
        {
          id: 'terminal',
          kind: 'terminal',
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
})
