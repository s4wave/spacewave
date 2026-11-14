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

const terminalEncoder = new TextEncoder()

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
    write: vi.fn<(data: string | Uint8Array) => void>(),
    writeln: vi.fn<(data: string) => void>(),
    dispose: vi.fn<() => void>(),
    fit: vi.fn<() => void>(),
    clientFrames: new Array<TerminalFrame>(),
    serverFrames: new Array<TerminalFrame>(),
    closeSeen: closeWaiter.promise,
    resolveClose: closeWaiter.resolve,
    abortObservedBeforeExit: true,
    connectCalls: 0,
    handleError: null as Error | null,
    handleRetry: vi.fn<() => void>(),
    watchError: null as Error | null,
    watchRetry: vi.fn<() => void>(),
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
    write(data: string | Uint8Array) {
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
    data: terminalEncoder.encode('ready\n'),
  }
  await h.closeSeen
  h.abortObservedBeforeExit =
    signal instanceof AbortSignal ? signal.aborted : true
  yield { kind: TerminalFrameKind.EXIT, exitCode: 0 }
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () =>
    h.handleError
      ? {
          value: null,
          loading: false,
          error: h.handleError,
          retry: h.handleRetry,
        }
      : handleResource(),
}))

function handleResource() {
  return {
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
  }
}

// The watch reports its parent's error ahead of its own and drops its value
// on a failure, as useStreamingResource does.
vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: (parent: {
    error: Error | null
    retry: () => void
  }) => {
    const error = parent.error ?? h.watchError
    return {
      value: error ? null : currentState,
      loading: false,
      error,
      retry: parent.error ? parent.retry : h.watchRetry,
    }
  },
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
    h.handleError = null
    h.watchError = null
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
    await vi.waitFor(() =>
      expect(h.write).toHaveBeenCalledWith(terminalEncoder.encode('ready\n')),
    )
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
    await vi.waitFor(() =>
      expect(h.write).toHaveBeenCalledWith(terminalEncoder.encode('ready\n')),
    )
    unmount()
  })

  it('draws a failed handle once, with one Retry that reopens it', () => {
    // A handle that failed to open draws its status alone.
    h.handleError = new Error('access denied')
    renderViewer()

    // The failure shows once, under the handle's message.
    expect(screen.getAllByRole('alert')).toHaveLength(1)
    expect(
      screen.getByText('Could not open the terminal: access denied'),
    ).toBeTruthy()

    // Its one Retry reopens the handle; no screen connects.
    const retries = screen.getAllByRole('button', { name: 'Retry' })
    expect(retries).toHaveLength(1)
    fireEvent.click(retries[0])
    expect(h.handleRetry).toHaveBeenCalledTimes(1)
    expect(h.open).not.toHaveBeenCalled()
  })

  it('shows a failed state watch as a notice above the running screen', async () => {
    // The handle opens but the state watch fails.
    h.watchError = new Error('stream reset')
    renderViewer()

    // The notice shows once, and the bar names no target it has not read.
    expect(screen.getAllByRole('alert')).toHaveLength(1)
    expect(
      screen.getByText('Could not read the session state: stream reset'),
    ).toBeTruthy()
    expect(screen.getByText('Terminal')).toBeTruthy()
    expect(screen.queryByText('devices/build-host')).toBeNull()

    // Its Retry restarts the watch while the screen still connects.
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(h.watchRetry).toHaveBeenCalledTimes(1)
    await vi.waitFor(() => expect(h.open).toHaveBeenCalled())
  })
})

// renderViewer renders the viewer on a Terminal object.
function renderViewer() {
  return render(
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
}
