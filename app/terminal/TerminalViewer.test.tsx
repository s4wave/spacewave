import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import {
  TerminalFrameKind,
  TerminalSessionState,
  type Terminal,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

const h = vi.hoisted(() => ({
  open: vi.fn<(host: HTMLElement) => void>(),
  write: vi.fn<(data: string) => void>(),
  writeln: vi.fn<(data: string) => void>(),
  dispose: vi.fn<() => void>(),
  fit: vi.fn<() => void>(),
  clientFrames: new Array<TerminalFrame>(),
}))

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

const currentState: Terminal = {
  name: 'Build Host Terminal',
  deviceObjectKey: 'devices/build-host',
  devicePeerId: '12D3KooWDevice',
  state: TerminalSessionState.DISCONNECTED,
  status: 'not connected',
  cols: 80,
  rows: 24,
}

async function* terminalFrames(): AsyncIterable<TerminalFrame> {
  await Promise.resolve()
  yield { kind: TerminalFrameKind.READY }
  yield {
    kind: TerminalFrameKind.OUTPUT,
    data: new TextEncoder().encode('ready\n'),
  }
}

vi.mock('@s4wave/web/hooks/useAccessTypedHandle.js', () => ({
  useAccessTypedHandle: () => ({
    value: {
      watchTerminalState: vi.fn(),
      connectTerminal: vi.fn(
        (frames: AsyncIterable<TerminalFrame>, signal?: AbortSignal) => {
          void (async () => {
            for await (const frame of frames) {
              if (signal?.aborted) return
              h.clientFrames.push(frame)
              if (frame.kind === TerminalFrameKind.CLOSE) return
            }
          })()
          return terminalFrames()
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
    expect(h.open).toHaveBeenCalled()
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
  })
})
