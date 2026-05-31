import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import {
  SshHostProbeState,
  type SshHost,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import {
  CreateTerminalOp,
  TerminalTargetKind,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'

const h = vi.hoisted(() => ({
  applyWorldOp: vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false }),
  navigateToObjects: vi.fn(),
  objects: [{ objectKey: 'terminal/prod-ssh-terminal-1' }],
}))

function defaultState(): SshHost {
  return {
    label: 'Prod SSH',
    endpoint: {
      host: 'prod.example.com',
      port: 2222,
      username: 'deploy',
    },
    credentials: {
      privateKeySecretObjectKey: 'secrets/ssh/prod-key',
      passphraseSecretObjectKey: 'secrets/ssh/prod-passphrase',
    },
    hostKeyPins: [
      {
        algorithm: 'ssh-ed25519',
        sha256Fingerprint: 'SHA256:example',
        acceptedByPeerId: '12D3KooWReviewer',
      },
    ],
    lastStatus: {
      state: SshHostProbeState.READY,
      message: 'probe ok',
    },
  }
}

let currentState: SshHost | undefined = defaultState()

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      watchSshHostState: vi.fn(),
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

import { SshHostViewer } from './SshHostViewer.js'

describe('SshHostViewer', () => {
  beforeEach(() => {
    currentState = defaultState()
    h.objects = [{ objectKey: 'terminal/prod-ssh-terminal-1' }]
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    currentState = undefined
  })

  it('renders SSH-only endpoint, trust, credential health, and probe status', () => {
    render(
      <SshHostViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'hosts/prod',
              objectType: 'spacewave/ssh-host',
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

    expect(screen.getByText('Prod SSH')).toBeTruthy()
    expect(screen.getByText('SSH-only')).toBeTruthy()
    expect(screen.getByText('Ready')).toBeTruthy()
    expect(screen.getByText('prod.example.com:2222')).toBeTruthy()
    expect(screen.getByText('deploy')).toBeTruthy()
    expect(screen.getByText('probe ok')).toBeTruthy()
    expect(screen.getByText('secrets/ssh/prod-key')).toBeTruthy()
    expect(screen.getByText('secrets/ssh/prod-passphrase')).toBeTruthy()
    expect(screen.getByText('SHA256:example')).toBeTruthy()
    expect(screen.getAllByText('Linked')).toHaveLength(2)
    expect(screen.getByText('Missing')).toBeTruthy()
    expect(screen.getByRole('button', { name: /open terminal/i })).toBeTruthy()
    const install = screen.getByRole('button', { name: /install agent/i })
    expect((install as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryByText('Online')).toBeNull()
    expect(screen.queryByText('Update')).toBeNull()
    expect(screen.queryByText(/Forge/)).toBeNull()
  })

  it('creates a Terminal object with an SSH Host target', async () => {
    render(
      <SshHostViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'hosts/prod',
              objectType: 'spacewave/ssh-host',
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

    fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))

    await waitFor(() =>
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
    expect(op.objectKey).toBe('prod-ssh-terminal-1')
    expect(op.name).toBe('Prod SSH Terminal')
    expect(op.targetKind).toBe(TerminalTargetKind.SSH_HOST)
    expect(op.sshHostObjectKey).toBe('hosts/prod')
    expect(op.deviceObjectKey ?? '').toBe('')
    expect(op.devicePeerId ?? '').toBe('')
    expect(h.navigateToObjects).toHaveBeenCalledWith(['prod-ssh-terminal-1'])
  })
})
