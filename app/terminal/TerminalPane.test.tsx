import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

const terminalEncoder = new TextEncoder()
const terminalDecoder = new TextDecoder()

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
    focus: vi.fn<() => void>(),
    write: vi.fn<(data: string) => void>(),
    writeln: vi.fn<(data: string) => void>(),
    dispose: vi.fn<() => void>(),
    inputDispose: vi.fn<() => void>(),
    fit: vi.fn<() => void>(),
    clientFrames: new Array<TerminalFrame>(),
    events: new Array<string>(),
    onDataCallbacks: new Array<(data: string) => void>(),
    closeSeen: closeWaiter.promise,
    resolveClose: closeWaiter.resolve,
  }
})

const resizeObservers: ResizeObserverMock[] = []

class ResizeObserverMock {
  private callback: ResizeObserverCallback
  private targets: Element[] = []

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
    resizeObservers.push(this)
  }

  observe(target: Element) {
    this.targets.push(target)
  }

  unobserve(target: Element) {
    this.targets = this.targets.filter((observed) => observed !== target)
  }

  disconnect() {
    this.targets = []
  }

  trigger(target: Element = this.targets[0] ?? document.body) {
    const rect = target.getBoundingClientRect()
    const entry: ResizeObserverEntry = {
      target,
      contentRect: new DOMRect(0, 0, rect.width, rect.height),
      borderBoxSize: [],
      contentBoxSize: [],
      devicePixelContentBoxSize: [],
    }
    this.callback([entry], this)
  }
}

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80
    rows = 24
    loadAddon() {}
    open(host: HTMLElement) {
      h.open(host)
    }
    focus() {
      h.focus()
    }
    onData(callback: (data: string) => void) {
      h.onDataCallbacks.push(callback)
      return {
        dispose() {
          h.events.push('input.dispose')
          h.inputDispose()
        },
      }
    }
    write(data: string) {
      h.write(data)
    }
    writeln(data: string) {
      h.writeln(data)
    }
    dispose() {
      h.events.push('terminal.dispose')
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

import {
  TerminalPane,
  safeTerminalFailureDetail,
  type TerminalPaneConnector,
  type TerminalPaneProps,
} from './TerminalPane.js'

async function collectClientFrames(
  frames: AsyncIterable<TerminalFrame>,
  signal: AbortSignal,
) {
  for await (const frame of frames) {
    if (signal.aborted) return
    h.clientFrames.push(frame)
    if (frame.kind === TerminalFrameKind.CLOSE) {
      h.events.push('client.close')
      h.resolveClose()
    }
  }
}

async function* terminalFrames(
  frames: TerminalFrame[],
  holdOpen = true,
): AsyncIterable<TerminalFrame> {
  await Promise.resolve()
  for (const frame of frames) {
    yield frame
  }
  if (holdOpen) {
    await h.closeSeen
  }
}

function renderTerminalPane(
  serverFrames: TerminalFrame[] = [],
  holdOpen = true,
  statusActions: Pick<TerminalPaneProps, 'onRetry' | 'onBackToSettings'> = {},
) {
  const connectTerminal: TerminalPaneConnector = (frames, signal) => {
    signal.addEventListener('abort', () => h.events.push('rpc.abort'))
    void collectClientFrames(frames, signal)
    return terminalFrames(serverFrames, holdOpen)
  }

  return render(
    <TerminalPane connectTerminal={connectTerminal} {...statusActions} />,
  )
}

describe('TerminalPane', () => {
  beforeEach(() => {
    resizeObservers.length = 0
    vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.clientFrames = []
    h.events = []
    h.onDataCallbacks = []
    h.closeSeen = new Promise<void>((resolve) => {
      h.resolveClose = resolve
    })
    vi.unstubAllGlobals()
  })

  it('sends decoded input bytes and resize frames on observed pane resize', async () => {
    const { unmount } = renderTerminalPane()

    await vi.waitFor(() =>
      expect(
        h.clientFrames.filter(
          (frame) => frame.kind === TerminalFrameKind.RESIZE,
        ),
      ).toHaveLength(1),
    )
    expect(h.clientFrames[0]).toMatchObject({
      kind: TerminalFrameKind.RESIZE,
      cols: 80,
      rows: 24,
    })

    h.onDataCallbacks[0]?.('echo ✓\r')

    await vi.waitFor(() =>
      expect(
        h.clientFrames.some(
          (frame) =>
            frame.kind === TerminalFrameKind.INPUT &&
            terminalDecoder.decode(frame.data) === 'echo ✓\r',
        ),
      ).toBe(true),
    )

    resizeObservers[0]?.trigger()

    await vi.waitFor(() =>
      expect(
        h.clientFrames.filter(
          (frame) => frame.kind === TerminalFrameKind.RESIZE,
        ),
      ).toHaveLength(2),
    )
    expect(h.fit).toHaveBeenCalledTimes(2)

    unmount()
  })
  it('disables page text selection without changing the xterm host', () => {
    const { container, unmount } = renderTerminalPane()
    const pane = container.querySelector('[data-terminal-state]')

    expect(pane?.classList.contains('select-none')).toBe(true)
    expect(pane?.firstElementChild?.classList.contains('select-none')).toBe(
      false,
    )

    unmount()
  })

  it('writes output frame bytes to xterm', async () => {
    const { unmount } = renderTerminalPane([
      {
        kind: TerminalFrameKind.OUTPUT,
        data: terminalEncoder.encode('deploy complete\n'),
      },
    ])

    await vi.waitFor(() =>
      expect(h.write).toHaveBeenCalledWith('deploy complete\n'),
    )
    unmount()
  })

  it('projects terminal transport errors into safe persistent status UI', async () => {
    const { unmount } = renderTerminalPane(
      [
        {
          kind: TerminalFrameKind.ERROR,
          error:
            'ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain',
        },
      ],
      false,
      { onRetry: vi.fn(), onBackToSettings: vi.fn() },
    )

    await vi.waitFor(() => {
      expect(screen.getByText('CLI session failed')).toBeDefined()
      expect(
        screen.getByText(
          'The SSH host rejected the username or credentials. Check the host settings, then retry.',
        ),
      ).toBeDefined()
    })
    expect(screen.queryByText('permission denied')).toBeNull()
    expect(h.writeln).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeDefined()
    expect(
      screen.getByRole('button', { name: 'Back to Settings' }),
    ).toBeDefined()

    unmount()
  })

  it('projects terminal exit frames into a closed restart state', async () => {
    const { unmount } = renderTerminalPane(
      [
        {
          kind: TerminalFrameKind.EXIT,
          exitCode: 137,
        },
      ],
      false,
      { onRetry: vi.fn() },
    )

    await vi.waitFor(() =>
      expect(screen.getByText('CLI session ended')).toBeDefined(),
    )
    expect(screen.getByRole('button', { name: 'Restart' })).toBeDefined()
    expect(h.writeln).not.toHaveBeenCalled()

    unmount()
  })

  it('renders connecting status until the first terminal output', async () => {
    const { container, unmount } = renderTerminalPane([
      { kind: TerminalFrameKind.READY },
      {
        kind: TerminalFrameKind.OUTPUT,
        data: terminalEncoder.encode('booting terminal\n'),
      },
    ])

    expect(screen.getByRole('status')).toBeDefined()
    expect(screen.getByText('Connecting to Spacewave CLI…')).toBeDefined()
    await vi.waitFor(() =>
      expect(container.querySelector('[data-terminal-state="ready"]')).not.toBe(
        null,
      ),
    )
    expect(screen.queryByRole('status')).toBeNull()
    expect(h.focus).toHaveBeenCalled()

    unmount()
  })

  it('bounds queued client frames while still delivering CLOSE on cleanup', async () => {
    let releaseQueue!: () => void
    const queueReleased = new Promise<void>((resolve) => {
      releaseQueue = resolve
    })
    const connectTerminal: TerminalPaneConnector = (frames, signal) => {
      signal.addEventListener('abort', () => h.events.push('rpc.abort'))
      void (async () => {
        const iterator = frames[Symbol.asyncIterator]()
        await queueReleased
        for (;;) {
          if (signal.aborted) return
          const next = await iterator.next()
          if (next.done) return
          h.clientFrames.push(next.value)
          if (next.value.kind === TerminalFrameKind.CLOSE) {
            h.events.push('client.close')
            h.resolveClose()
          }
        }
      })()
      return terminalFrames([], true)
    }
    const { unmount } = render(
      <TerminalPane connectTerminal={connectTerminal} />,
    )

    await vi.waitFor(() => expect(h.onDataCallbacks).toHaveLength(1))

    for (let i = 0; i < 300; i++) {
      h.onDataCallbacks[0]?.(`queued-${String(i).padStart(3, '0')}\r`)
    }
    releaseQueue()

    await vi.waitFor(() =>
      expect(
        h.clientFrames.filter(
          (frame) => frame.kind === TerminalFrameKind.INPUT,
        ),
      ).toHaveLength(256),
    )

    const inputFrames = h.clientFrames.filter(
      (frame) => frame.kind === TerminalFrameKind.INPUT,
    )
    expect(inputFrames).toHaveLength(256)
    expect(terminalDecoder.decode(inputFrames[0]?.data)).toBe('queued-044\r')
    expect(terminalDecoder.decode(inputFrames.at(-1)?.data)).toBe(
      'queued-299\r',
    )

    unmount()

    await vi.waitFor(() => expect(h.events).toContain('rpc.abort'))
    expect(h.clientFrames.at(-1)).toEqual({ kind: TerminalFrameKind.CLOSE })
  })

  it('sends CLOSE before aborting RPC and disposes xterm resources during cleanup', async () => {
    const { unmount } = renderTerminalPane()

    await vi.waitFor(() =>
      expect(
        h.clientFrames.some((frame) => frame.kind === TerminalFrameKind.RESIZE),
      ).toBe(true),
    )

    unmount()

    await vi.waitFor(() => expect(h.events).toContain('rpc.abort'))

    const closeFrame = h.clientFrames.find(
      (frame) => frame.kind === TerminalFrameKind.CLOSE,
    )
    expect(closeFrame).toEqual({ kind: TerminalFrameKind.CLOSE })

    const closeIndex = h.events.indexOf('client.close')
    expect(closeIndex).toBeGreaterThanOrEqual(0)
    expect(closeIndex).toBeLessThan(h.events.indexOf('rpc.abort'))
    expect(h.inputDispose).toHaveBeenCalled()
    expect(h.dispose).toHaveBeenCalled()
  })
  it.each([
    [
      'ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain',
      'The SSH host rejected the username or credentials. Check the host settings, then retry.',
    ],
    [
      'dial tcp 10.0.0.8:22: connect: connection refused',
      'The SSH host refused the connection. Check that SSH is running and the host and port are correct.',
    ],
    [
      'dial tcp: lookup host.internal: no such host',
      'The terminal session could not start. Check the host settings, then retry.',
    ],
    [
      'permission denied while reading object',
      'The terminal session could not start. Check the host settings, then retry.',
    ],
  ])('safely classifies SSH producer failure %s', (raw, expected) => {
    expect(safeTerminalFailureDetail(raw)).toBe(expected)
  })

  it('lets an explicit retry callback replace the local reconnect', async () => {
    const onRetry = vi.fn()
    const connectTerminal = vi.fn<TerminalPaneConnector>(() =>
      terminalFrames(
        [{ kind: TerminalFrameKind.ERROR, error: 'connection refused' }],
        false,
      ),
    )
    render(<TerminalPane connectTerminal={connectTerminal} onRetry={onRetry} />)

    await screen.findByText('CLI session failed')
    screen.getByRole('button', { name: 'Retry' }).click()

    expect(onRetry).toHaveBeenCalledOnce()
    await Promise.resolve()
    expect(connectTerminal).toHaveBeenCalledOnce()
  })

  it('starts only one connector under StrictMode', async () => {
    const connectTerminal = vi.fn<TerminalPaneConnector>(() =>
      terminalFrames([], true),
    )
    render(
      <React.StrictMode>
        <TerminalPane connectTerminal={connectTerminal} />
      </React.StrictMode>,
    )

    await vi.waitFor(() => expect(connectTerminal).toHaveBeenCalledOnce())
  })
  it('waits for CLOSE delivery and abort before starting a retry connector', async () => {
    let releaseClose!: () => void
    const closeReleased = new Promise<void>((resolve) => {
      releaseClose = resolve
    })
    let calls = 0
    const connectTerminal: TerminalPaneConnector = (frames, signal) => {
      calls += 1
      h.events.push(`connect.${calls}`)
      signal.addEventListener('abort', () => h.events.push(`abort.${calls}`))
      void (async () => {
        const iterator = frames[Symbol.asyncIterator]()
        if (calls === 1) await closeReleased
        for (;;) {
          const next = await iterator.next()
          if (next.done || signal.aborted) return
          h.clientFrames.push(next.value)
          if (next.value.kind === TerminalFrameKind.CLOSE) {
            h.events.push('client.close')
            h.resolveClose()
          }
        }
      })()
      return terminalFrames(
        [{ kind: TerminalFrameKind.ERROR, error: 'connection refused' }],
        false,
      )
    }
    render(<TerminalPane connectTerminal={connectTerminal} />)

    await screen.findByText('CLI session failed')
    screen.getByRole('button', { name: 'Retry' }).click()
    await Promise.resolve()
    expect(h.events).toEqual(['connect.1'])

    releaseClose()
    await vi.waitFor(() => expect(h.events).toContain('connect.2'))
    expect(h.events.indexOf('client.close')).toBeLessThan(
      h.events.indexOf('abort.1'),
    )
    expect(h.events.indexOf('abort.1')).toBeLessThan(
      h.events.indexOf('connect.2'),
    )
  })
})
