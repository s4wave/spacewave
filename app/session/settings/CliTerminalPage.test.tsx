import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'

const cliPageMocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  buildWebViewHostOpenStream: vi.fn((webViewUuid: string) => ({
    webViewUuid,
  })),
  srpcClient: vi.fn(function (openStream: unknown) {
    return { openStream }
  }),
  buildRpcStreamOpenStream: vi.fn(function (
    serviceId: string,
    pluginRpc: unknown,
  ) {
    return { serviceId, pluginRpc }
  }),
  pluginRpc: vi.fn(),
  loadPlugin: vi.fn(),
  loadPluginRequests: [] as Array<{ pluginId?: string; instanceKey?: string }>,
  loadPluginSignals: [] as AbortSignal[],
  loadPluginClosed: vi.fn(),
  runCli: vi.fn(),
  runCliInputFrames: [] as TerminalFrame[],
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
  buildRpcStreamOpenStream: cliPageMocks.buildRpcStreamOpenStream,
}))

vi.mock(
  '@go/github.com/s4wave/spacewave/bldr/plugin/plugin_srpc.pb.js',
  () => ({
    PluginHostServiceName: 'PluginHost',
    PluginHostClient: vi.fn(function () {
      return {
        LoadPlugin: cliPageMocks.loadPlugin,
        PluginRpc: cliPageMocks.pluginRpc,
      }
    }),
  }),
)

vi.mock('@s4wave/sdk/cli/terminal/terminal_srpc.pb.js', () => ({
  CliTerminalServiceClient: vi.fn(function () {
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
    cliPageMocks.buildRpcStreamOpenStream.mockClear()
    cliPageMocks.pluginRpc.mockClear()
    cliPageMocks.loadPlugin.mockReset()
    cliPageMocks.loadPluginRequests.length = 0
    cliPageMocks.loadPluginSignals.length = 0
    cliPageMocks.loadPluginClosed.mockReset()
    cliPageMocks.runCli.mockReset()
    cliPageMocks.runCliInputFrames.length = 0
    cliPageMocks.terminalConnector = null

    cliPageMocks.loadPlugin.mockImplementation(async function* (
      request: { pluginId?: string; instanceKey?: string },
      signal: AbortSignal,
    ) {
      cliPageMocks.loadPluginRequests.push(request)
      cliPageMocks.loadPluginSignals.push(signal)
      try {
        yield { pluginStatus: { running: true } }
        await waitForAbort(signal)
      } finally {
        cliPageMocks.loadPluginClosed()
      }
    })
    cliPageMocks.runCli.mockImplementation(async function* (
      frames: AsyncIterable<TerminalFrame>,
      signal: AbortSignal,
    ) {
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

  it('loads the CLI plugin, streams terminal frames through RunCli, and aborts the plugin lease on close', async () => {
    render(<CliTerminalPage />)

    expect(screen.getByTestId('cli-terminal-pane').textContent).toBe(
      'terminal connector mounted',
    )
    expect(screen.getByText('CLI terminal')).toBeDefined()
    expect(screen.getByText('Session 7')).toBeDefined()

    fireEvent.click(
      screen.getByRole('button', { name: 'Back to Command Line' }),
    )
    expect(cliPageMocks.navigate).toHaveBeenCalledWith({ path: '../' })

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
    expect(cliPageMocks.loadPluginRequests).toHaveLength(1)
    expect(cliPageMocks.loadPluginRequests[0]).toMatchObject({
      pluginId: 'spacewave-cli-plugin',
    })
    expect(cliPageMocks.loadPluginRequests[0]?.instanceKey).toMatch(
      /^cli-terminal\//,
    )
    expect(cliPageMocks.buildRpcStreamOpenStream).toHaveBeenCalledWith(
      'spacewave-cli-plugin/' + cliPageMocks.loadPluginRequests[0]?.instanceKey,
      cliPageMocks.pluginRpc,
    )
    expect(cliPageMocks.runCliInputFrames).toEqual([inputFrame])
    expect(outputFrames).toEqual([
      { kind: TerminalFrameKind.OUTPUT, data: new Uint8Array([111, 107]) },
    ])
    await waitFor(() => {
      expect(cliPageMocks.loadPluginSignals[0]?.aborted).toBe(true)
      expect(cliPageMocks.loadPluginClosed).toHaveBeenCalledTimes(1)
    })
  })
})
