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

import { DeviceViewer } from './DeviceViewer.js'

describe('DeviceViewer', () => {
  afterEach(() => {
    cleanup()
    currentState = undefined
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
          value: {} as never,
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
  })
})
