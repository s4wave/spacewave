import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

import {
  TerminalFrameKind,
  TerminalSessionState,
  TerminalTargetKind,
  type Terminal,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

const h = vi.hoisted(() => {
  const closeWaiter = {
    promise: Promise.resolve(),
    resolve: () => {},
  }
  closeWaiter.promise = new Promise<void>((resolve) => {
    closeWaiter.resolve = resolve
  })
  return {
    open: vi.fn<(host: HTMLElement) => void>(),
    write: vi.fn<(data: string) => void>(),
    writeln: vi.fn<(data: string) => void>(),
    dispose: vi.fn<() => void>(),
    fit: vi.fn<() => void>(),
    clientFrames: new Array<TerminalFrame>(),
    serverFrames: new Array<TerminalFrame>(),
    closeSeen: closeWaiter.promise,
    resolveClose: closeWaiter.resolve,
    abortObservedBeforeExit: true,
    connectCalls: 0,
  }
})

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80
    rows = 24
    loadAddon() {}
    open(host: HTMLElement) {
      h.open(host)
    }
    onData() {
      return { dispose: vi.fn() }
    }
    write(data: string) {
      h.write(data)
    }
    writeln(data: string) {
      h.writeln(data)
    }
    dispose() {
      h.dispose()
    }
  },
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {
      h.fit()
    }
  },
}))

let currentState: Terminal = {
  name: 'Build Host Terminal',
  deviceObjectKey: 'devices/build-host',
  devicePeerId: '12D3KooWDevice',
  targetKind: TerminalTargetKind.DEVICE,
  state: TerminalSessionState.DISCONNECTED,
  status: 'not connected',
  cols: 80,
  rows: 24,
}

async function* terminalFrames(
  signal?: AbortSignal,
): AsyncIterable<TerminalFrame> {
  await Promise.resolve()
  for (const frame of h.serverFrames) {
    yield frame
  }
  if (
    h.serverFrames.some(
      (frame) => frame.kind === TerminalFrameKind.SSH_HOST_KEY_TRUST_CHALLENGE,
    )
  ) {
    await h.closeSeen
    return
  }
  yield { kind: TerminalFrameKind.READY }
  yield {
    kind: TerminalFrameKind.OUTPUT,
    data: new TextEncoder().encode('ready\n'),
  }
  await h.closeSeen
  h.abortObservedBeforeExit =
    signal instanceof AbortSignal ? signal.aborted : true
  yield { kind: TerminalFrameKind.EXIT, exitCode: 0 }
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      watchTerminalState: vi.fn(),
      connectTerminal: vi.fn(
        (frames: AsyncIterable<TerminalFrame>, signal?: AbortSignal) => {
          h.connectCalls += 1
          void (async () => {
            for await (const frame of frames) {
              if (signal?.aborted) return
              h.clientFrames.push(frame)
              if (frame.kind === TerminalFrameKind.CLOSE) {
                h.abortObservedBeforeExit = signal?.aborted === true
                h.resolveClose()
                continue
              }
            }
          })()
          return terminalFrames(signal)
        },
      ),
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

import { TerminalViewer } from './TerminalViewer.js'

describe('TerminalViewer', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.clientFrames = []
    h.serverFrames = []
    h.closeSeen = new Promise<void>((resolve) => {
      h.resolveClose = resolve
    })
    h.abortObservedBeforeExit = true
    h.connectCalls = 0
    currentState = {
      name: 'Build Host Terminal',
      deviceObjectKey: 'devices/build-host',
      devicePeerId: '12D3KooWDevice',
      targetKind: TerminalTargetKind.DEVICE,
      state: TerminalSessionState.DISCONNECTED,
      status: 'not connected',
      cols: 80,
      rows: 24,
    }
  })

  it('renders terminal state and opens xterm host', async () => {
    const { unmount } = render(
      <TerminalViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'terminal/build-host',
              objectType: 'spacewave/terminal',
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

    expect(screen.getByText('Build Host Terminal')).toBeTruthy()
    expect(screen.getByText('Disconnected')).toBeTruthy()
    expect(screen.getByText('devices/build-host')).toBeTruthy()
    await vi.waitFor(() => expect(h.open).toHaveBeenCalled())
    await vi.waitFor(() => expect(h.write).toHaveBeenCalledWith('ready\n'))
    await vi.waitFor(() =>
      expect(h.clientFrames[0]?.kind).toBe(TerminalFrameKind.RESIZE),
    )
    unmount()
    await vi.waitFor(() =>
      expect(
        h.clientFrames.some((frame) => frame.kind === TerminalFrameKind.CLOSE),
      ).toBe(true),
    )
    await vi.waitFor(() => expect(h.abortObservedBeforeExit).toBe(false))
  })

  it('labels SSH Host terminal targets separately from Device targets', () => {
    currentState = {
      name: 'Prod SSH Terminal',
      sshHostObjectKey: 'hosts/prod',
      targetKind: TerminalTargetKind.SSH_HOST,
      state: TerminalSessionState.DISCONNECTED,
      status: 'not connected',
      cols: 80,
      rows: 24,
    }

    render(
      <TerminalViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'terminal/prod-ssh',
              objectType: 'spacewave/terminal',
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

    expect(screen.getByText('Prod SSH Terminal')).toBeTruthy()
    expect(screen.getByText('ssh host')).toBeTruthy()
    expect(screen.getByText('hosts/prod')).toBeTruthy()
  })

  it('renders SSH host trust challenge and sends accepted response', async () => {
    currentState = {
      name: 'Prod SSH Terminal',
      sshHostObjectKey: 'hosts/prod',
      targetKind: TerminalTargetKind.SSH_HOST,
      state: TerminalSessionState.CONNECTING,
      status: 'connecting',
      cols: 80,
      rows: 24,
    }
    h.serverFrames = [
      {
        kind: TerminalFrameKind.SSH_HOST_KEY_TRUST_CHALLENGE,
        sshTrustHost: 'prod.internal',
        sshTrustAlgorithm: 'ssh-ed25519',
        sshTrustSha256Fingerprint: 'SHA256:abc123',
        sshTrustPublicKey: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID',
      },
    ]

    const { unmount } = render(
      <TerminalViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'terminal/prod-ssh',
              objectType: 'spacewave/terminal',
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

    await screen.findByRole('dialog', { name: 'SSH host key trust' })
    expect(screen.getByText('prod.internal')).toBeTruthy()
    expect(screen.getByText('ssh-ed25519')).toBeTruthy()
    expect(screen.getByText('SHA256:abc123')).toBeTruthy()
    expect(
      screen.getByText('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID'),
    ).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /trust/i }))

    await vi.waitFor(() =>
      expect(
        h.clientFrames.some(
          (frame) =>
            frame.kind === TerminalFrameKind.SSH_HOST_KEY_TRUST_RESPONSE &&
            frame.sshTrustAccepted === true,
        ),
      ).toBe(true),
    )
    unmount()
  })

  it('sends rejected SSH host trust response', async () => {
    h.serverFrames = [
      {
        kind: TerminalFrameKind.SSH_HOST_KEY_TRUST_CHALLENGE,
        sshTrustHost: 'prod.internal',
        sshTrustAlgorithm: 'ssh-ed25519',
        sshTrustSha256Fingerprint: 'SHA256:abc123',
        sshTrustPublicKey: 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID',
      },
    ]

    const { unmount } = render(
      <TerminalViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'terminal/prod-ssh',
              objectType: 'spacewave/terminal',
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

    await screen.findByRole('dialog', { name: 'SSH host key trust' })
    fireEvent.click(screen.getByRole('button', { name: /reject/i }))

    await vi.waitFor(() =>
      expect(
        h.clientFrames.some(
          (frame) =>
            frame.kind === TerminalFrameKind.SSH_HOST_KEY_TRUST_RESPONSE &&
            frame.sshTrustAccepted === false,
        ),
      ).toBe(true),
    )
    unmount()
  })
  it('shows a safe SSH failure cause and Retry starts a new terminal attempt', async () => {
    currentState = {
      name: 'Prod SSH Terminal',
      sshHostObjectKey: 'hosts/prod',
      targetKind: TerminalTargetKind.SSH_HOST,
      state: TerminalSessionState.FAILED,
      status: 'failed to connect',
      error:
        'ssh: handshake failed: unable to authenticate, attempted methods [none password]',
      cols: 80,
      rows: 24,
    }
    h.serverFrames = [
      {
        kind: TerminalFrameKind.ERROR,
        error:
          'ssh: handshake failed: unable to authenticate, attempted methods [none password]',
      },
    ]

    const { unmount } = render(
      <TerminalViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'terminal/prod-ssh',
              objectType: 'spacewave/terminal',
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

    await screen.findByText('Terminal session failed')
    expect(
      screen.getAllByText(
        'The SSH host rejected the username or credentials. Check the host settings, then retry.',
      ),
    ).toHaveLength(1)
    expect(screen.queryByText(/unable to authenticate/i)).toBeNull()
    expect(h.connectCalls).toBe(1)

    h.serverFrames = []
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    await vi.waitFor(() => expect(h.connectCalls).toBe(2))
    await vi.waitFor(() => expect(h.write).toHaveBeenCalledWith('ready\n'))
    unmount()
  })
})
