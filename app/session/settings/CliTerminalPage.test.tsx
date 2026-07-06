import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'

const cliPageMocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  buildWebViewHostOpenStream: vi.fn((webViewUuid: string) => ({
    webViewUuid,
  })),
  srpcClient: vi.fn(function (openStream: unknown) {
    return { openStream }
  }),
  cliTerminalServiceName: 'CliTerminalService',
  cliTerminalClientArgs: [] as Array<{ rpcClient: unknown; options: unknown }>,
  runCli: vi.fn(),
  runCliInputFrames: [] as TerminalFrame[],
  runCliSignals: [] as AbortSignal[],
  terminalConnector: null as unknown,
}))

vi.mock('@aptre/bldr-react', () => ({
  useBldrContext: () => ({
    webDocument: {
      buildWebViewHostOpenStream: cliPageMocks.buildWebViewHostOpenStream,
    },
    webView: {
      getUuid: () => 'web-view-7',
    },
  }),
}))

vi.mock('starpc', () => ({
  Client: cliPageMocks.srpcClient,
}))

vi.mock('@s4wave/sdk/cli/terminal/terminal_srpc.pb.js', () => ({
  CliTerminalServiceServiceName: cliPageMocks.cliTerminalServiceName,
  CliTerminalServiceClient: vi.fn(function (
    rpcClient: unknown,
    options: unknown,
  ) {
    cliPageMocks.cliTerminalClientArgs.push({ rpcClient, options })
    return {
      RunCli: cliPageMocks.runCli,
    }
  }),
}))

vi.mock('@s4wave/app/terminal/TerminalPane.js', () => ({
  TerminalPane: ({ connectTerminal }: { connectTerminal: unknown }) => {
    cliPageMocks.terminalConnector = connectTerminal
    return <div data-testid="cli-terminal-pane">terminal connector mounted</div>
  },
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  useSessionIndex: () => 7,
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => cliPageMocks.navigate,
}))

import { CliTerminalPage } from './CliTerminalPage.js'

function waitForAbort(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  const { promise, resolve } = Promise.withResolvers<void>()
  signal.addEventListener('abort', () => resolve(), { once: true })
  return promise
}

describe('CliTerminalPage', () => {
  beforeEach(() => {
    cliPageMocks.navigate.mockReset()
    cliPageMocks.buildWebViewHostOpenStream.mockClear()
    cliPageMocks.srpcClient.mockClear()
    cliPageMocks.cliTerminalClientArgs.length = 0
    cliPageMocks.runCli.mockReset()
    cliPageMocks.runCliInputFrames.length = 0
    cliPageMocks.runCliSignals.length = 0
    cliPageMocks.terminalConnector = null

    cliPageMocks.runCli.mockImplementation(async function* (
      frames: AsyncIterable<TerminalFrame>,
      signal: AbortSignal,
    ) {
      cliPageMocks.runCliSignals.push(signal)
      for await (const frame of frames) {
        cliPageMocks.runCliInputFrames.push(frame)
        yield {
          kind: TerminalFrameKind.OUTPUT,
          data: new Uint8Array([111, 107]),
        }
      }
      await waitForAbort(signal)
    })
  })

  afterEach(() => cleanup())

  it('streams terminal frames through the CLI plugin service route and aborts RunCli on close', async () => {
    render(
      <BottomBarRoot>
        <CliTerminalPage />
      </BottomBarRoot>,
    )

    expect(screen.getByTestId('cli-terminal-pane').textContent).toBe(
      'terminal connector mounted',
    )
    expect(screen.getByRole('button', { name: /Terminal\s+CLI/ })).toBeDefined()
    expect(
      screen.queryByRole('button', { name: 'Back to Command Line' }),
    ).toBeNull()
    expect(screen.queryByText('CLI terminal')).toBeNull()
    expect(screen.queryByText('Session 7')).toBeNull()

    const connectTerminal = cliPageMocks.terminalConnector as (
      frames: AsyncIterable<TerminalFrame>,
      signal: AbortSignal,
    ) => AsyncIterable<TerminalFrame>
    const inputFrame: TerminalFrame = {
      kind: TerminalFrameKind.INPUT,
      data: new Uint8Array([104, 105]),
    }
    async function* inputFrames() {
      yield inputFrame
    }

    const controller = new AbortController()
    const outputFrames: TerminalFrame[] = []
    for await (const frame of connectTerminal(
      inputFrames(),
      controller.signal,
    )) {
      outputFrames.push(frame)
      controller.abort()
    }

    expect(cliPageMocks.buildWebViewHostOpenStream).toHaveBeenCalledWith(
      'web-view-7',
    )
    expect(cliPageMocks.srpcClient).toHaveBeenCalledWith({
      webViewUuid: 'web-view-7',
    })
    expect(cliPageMocks.cliTerminalClientArgs).toEqual([
      {
        rpcClient: { openStream: { webViewUuid: 'web-view-7' } },
        options: {
          service: `plugin/spacewave-cli-plugin/${cliPageMocks.cliTerminalServiceName}`,
        },
      },
    ])
    expect(cliPageMocks.runCliSignals).toEqual([controller.signal])
    expect(cliPageMocks.runCliInputFrames).toEqual([inputFrame])
    expect(outputFrames).toEqual([
      { kind: TerminalFrameKind.OUTPUT, data: new Uint8Array([111, 107]) },
    ])
    await waitFor(() => {
      expect(cliPageMocks.runCliSignals[0]?.aborted).toBe(true)
    })
  })
})
