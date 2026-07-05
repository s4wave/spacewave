import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render } from '@testing-library/react'

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

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80
    rows = 24
    loadAddon() {}
    open(host: HTMLElement) {
      h.open(host)
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

import { TerminalPane, type TerminalPaneConnector } from './TerminalPane.js'

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
) {
  const connectTerminal: TerminalPaneConnector = (frames, signal) => {
    signal.addEventListener('abort', () => h.events.push('rpc.abort'))
    void collectClientFrames(frames, signal)
    return terminalFrames(serverFrames, holdOpen)
  }

  return render(<TerminalPane connectTerminal={connectTerminal} />)
}

describe('TerminalPane', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
    h.clientFrames = []
    h.events = []
    h.onDataCallbacks = []
    h.closeSeen = new Promise<void>((resolve) => {
      h.resolveClose = resolve
    })
  })

  it('sends decoded input bytes and resize frames to the terminal stream', async () => {
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

    window.dispatchEvent(new Event('resize'))

    await vi.waitFor(() =>
      expect(
        h.clientFrames.filter(
          (frame) => frame.kind === TerminalFrameKind.RESIZE,
        ),
      ).toHaveLength(2),
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

  it('writes terminal error frames as terminal lines', async () => {
    const { unmount } = renderTerminalPane(
      [
        {
          kind: TerminalFrameKind.ERROR,
          error: 'permission denied',
        },
      ],
      false,
    )

    await vi.waitFor(() =>
      expect(h.writeln).toHaveBeenCalledWith('\r\npermission denied'),
    )

    unmount()
  })

  it('writes terminal exit frames as terminal lines', async () => {
    const { unmount } = renderTerminalPane(
      [
        {
          kind: TerminalFrameKind.EXIT,
          exitCode: 137,
        },
      ],
      false,
    )

    await vi.waitFor(() =>
      expect(h.writeln).toHaveBeenCalledWith('\r\nprocess exited 137'),
    )

    unmount()
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
})
