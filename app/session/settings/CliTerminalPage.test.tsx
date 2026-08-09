import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { CliTerminalSessionProvider } from '@s4wave/app/terminal/CliTerminalSessionProvider.js'

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

import { asyncValues } from '@s4wave/web/test/async-values.js'

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

  it('streams terminal frames through the persistent CLI session until its owner unmounts', async () => {
    const { unmount } = render(
      <CliTerminalSessionProvider>
        <BottomBarRoot>
          <CliTerminalPage />
        </BottomBarRoot>
      </CliTerminalSessionProvider>,
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
    const inputFrames = asyncValues(inputFrame)

    const controller = new AbortController()
    const outputFrames: TerminalFrame[] = []
    for await (const frame of connectTerminal(inputFrames, controller.signal)) {
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
    expect(cliPageMocks.runCliSignals).toHaveLength(1)
    expect(cliPageMocks.runCliSignals[0]).not.toBe(controller.signal)
    expect(cliPageMocks.runCliSignals[0]?.aborted).toBe(false)
    expect(cliPageMocks.runCliInputFrames).toEqual([inputFrame])
    expect(outputFrames).toEqual([
      { kind: TerminalFrameKind.OUTPUT, data: new Uint8Array([111, 107]) },
    ])

    unmount()

    await waitFor(() => {
      expect(cliPageMocks.runCliSignals[0]?.aborted).toBe(true)
    })
  })

  it('keeps a running command stream alive across pane unmount and remount', async () => {
    const commandStarted = Promise.withResolvers<void>()
    const finishCommand = Promise.withResolvers<void>()
    cliPageMocks.runCli.mockImplementation(async function* (
      frames: AsyncIterable<TerminalFrame>,
      signal: AbortSignal,
    ) {
      cliPageMocks.runCliSignals.push(signal)
      for await (const frame of frames) {
        cliPageMocks.runCliInputFrames.push(frame)
        commandStarted.resolve()
        await finishCommand.promise
        yield {
          kind: TerminalFrameKind.OUTPUT,
          data: new Uint8Array([100, 111, 110, 101]),
        }
      }
      await waitForAbort(signal)
    })

    function Harness({ showPane }: { showPane: boolean }) {
      return (
        <CliTerminalSessionProvider>
          <BottomBarRoot>{showPane ? <CliTerminalPage /> : null}</BottomBarRoot>
        </CliTerminalSessionProvider>
      )
    }

    const view = render(<Harness showPane={true} />)
    const firstConnector = cliPageMocks.terminalConnector as (
      frames: AsyncIterable<TerminalFrame>,
      signal: AbortSignal,
    ) => AsyncIterable<TerminalFrame>
    const firstInput: TerminalFrame = {
      kind: TerminalFrameKind.INPUT,
      data: new Uint8Array([108, 111, 110, 103, 45, 114, 117, 110]),
    }
    const closeFirstPane = Promise.withResolvers<void>()
    async function* firstInputFrames() {
      yield firstInput
      await closeFirstPane.promise
      yield { kind: TerminalFrameKind.CLOSE }
    }
    const firstPane = new AbortController()
    const firstOutput = firstConnector(firstInputFrames(), firstPane.signal)[
      Symbol.asyncIterator
    ]()
    const firstRead = firstOutput.next()

    await commandStarted.promise
    closeFirstPane.resolve()
    expect(await firstRead).toEqual({
      done: true,
      value: undefined,
    })
    firstPane.abort()

    expect(cliPageMocks.runCliSignals).toHaveLength(1)
    expect(cliPageMocks.runCliSignals[0]?.aborted).toBe(false)

    view.rerender(<Harness showPane={false} />)
    view.rerender(<Harness showPane={true} />)

    const secondConnector =
      cliPageMocks.terminalConnector as typeof firstConnector
    const secondPane = new AbortController()
    const secondInputFrames: AsyncIterable<TerminalFrame> = {
      [Symbol.asyncIterator]: () => ({
        next: async (): Promise<IteratorResult<TerminalFrame>> => {
          await waitForAbort(secondPane.signal)
          return { done: true, value: undefined }
        },
      }),
    }
    const secondOutput = secondConnector(secondInputFrames, secondPane.signal)[
      Symbol.asyncIterator
    ]()
    const secondRead = secondOutput.next()

    finishCommand.resolve()
    expect(await secondRead).toEqual({
      done: false,
      value: {
        kind: TerminalFrameKind.OUTPUT,
        data: new Uint8Array([100, 111, 110, 101]),
      },
    })
    expect(cliPageMocks.runCli).toHaveBeenCalledTimes(1)
    expect(cliPageMocks.runCliInputFrames).toEqual([firstInput])
    expect(cliPageMocks.runCliSignals[0]?.aborted).toBe(false)

    secondPane.abort()
    await secondOutput.return?.()
  })

  it('replaces a terminal stream that ended before a pane reattaches', async () => {
    cliPageMocks.runCli.mockImplementationOnce(
      (_frames: AsyncIterable<TerminalFrame>, signal: AbortSignal) => {
        cliPageMocks.runCliSignals.push(signal)
        return asyncValues<TerminalFrame>({
          kind: TerminalFrameKind.ERROR,
          error: 'first-stream-ended',
        })
      },
    )

    function Harness({ showPane }: { showPane: boolean }) {
      return (
        <CliTerminalSessionProvider>
          <BottomBarRoot>{showPane ? <CliTerminalPage /> : null}</BottomBarRoot>
        </CliTerminalSessionProvider>
      )
    }

    const view = render(<Harness showPane={true} />)
    const firstConnector = cliPageMocks.terminalConnector as (
      frames: AsyncIterable<TerminalFrame>,
      signal: AbortSignal,
    ) => AsyncIterable<TerminalFrame>
    const firstPane = new AbortController()
    const firstOutput = firstConnector(
      (async function* () {})(),
      firstPane.signal,
    )[Symbol.asyncIterator]()

    expect(await firstOutput.next()).toEqual({
      done: false,
      value: {
        kind: TerminalFrameKind.ERROR,
        error: 'first-stream-ended',
      },
    })
    expect(await firstOutput.next()).toEqual({
      done: true,
      value: undefined,
    })

    view.rerender(<Harness showPane={false} />)
    view.rerender(<Harness showPane={true} />)

    const secondConnector =
      cliPageMocks.terminalConnector as typeof firstConnector
    const secondInput: TerminalFrame = {
      kind: TerminalFrameKind.INPUT,
      data: new Uint8Array([114, 101, 116, 114, 121]),
    }
    const secondInputFrames = asyncValues(secondInput)
    const secondPane = new AbortController()
    const secondOutput = secondConnector(secondInputFrames, secondPane.signal)[
      Symbol.asyncIterator
    ]()

    expect(await secondOutput.next()).toEqual({
      done: false,
      value: {
        kind: TerminalFrameKind.OUTPUT,
        data: new Uint8Array([111, 107]),
      },
    })
    expect(cliPageMocks.runCli).toHaveBeenCalledTimes(2)
    expect(cliPageMocks.runCliSignals).toHaveLength(2)
    expect(cliPageMocks.runCliSignals[1]?.aborted).toBe(false)

    secondPane.abort()
    await secondOutput.return?.()
  })
})
