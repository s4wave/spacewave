import { useCallback, useMemo } from 'react'
import { useBldrContext } from '@aptre/bldr-react'
import { buildRpcStreamOpenStream, Client as SRPCClient } from 'starpc'
import type { MessageStream } from 'starpc'
import { LuArrowLeft, LuTerminal } from 'react-icons/lu'
import {
  PluginHostClient,
  PluginHostServiceName,
  type PluginHost,
} from '@go/github.com/s4wave/spacewave/bldr/plugin/plugin_srpc.pb.js'

import { CliTerminalServiceClient } from '@s4wave/sdk/cli/terminal/terminal_srpc.pb.js'
import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import {
  TerminalPane,
  type TerminalPaneConnector,
} from '@s4wave/app/terminal/TerminalPane.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'

const cliTerminalPluginId = 'spacewave-cli-plugin'
const pluginHostServiceId = 'plugin-host/' + PluginHostServiceName

interface PluginLoadLease {
  running: Promise<void>
  done: Promise<void>
}

// CliTerminalPage renders the session-local in-app CLI terminal.
export function CliTerminalPage() {
  const navigate = useNavigate()
  const sessionIdx = useSessionIndex()
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument ?? null
  const webViewUuid = bldrContext?.webView?.getUuid() ?? null
  const instanceKey = useMemo(() => 'cli-terminal/' + crypto.randomUUID(), [])

  const pluginHost = useMemo<PluginHost | null>(() => {
    if (!webDocument || !webViewUuid) return null
    const rpcClient = new SRPCClient(
      webDocument.buildWebViewHostOpenStream(webViewUuid),
    )
    return new PluginHostClient(rpcClient, { service: pluginHostServiceId })
  }, [webDocument, webViewUuid])

  const connectTerminal = useMemo<TerminalPaneConnector>(() => {
    if (!pluginHost) {
      return async function* (): MessageStream<TerminalFrame> {
        yield {
          kind: TerminalFrameKind.ERROR,
          error: 'Spacewave runtime context is unavailable.',
        }
      }
    }
    return (frames, signal) =>
      runCliTerminal(pluginHost, instanceKey, frames, signal)
  }, [instanceKey, pluginHost])

  const handleBack = useCallback(() => {
    navigate({ path: '../' })
  }, [navigate])

  return (
    <div className="bg-background-landing flex min-h-0 flex-1 flex-col">
      <div className="border-foreground/10 flex shrink-0 items-center gap-3 border-b px-6 py-4 md:px-10">
        <button
          onClick={handleBack}
          className="text-foreground-alt hover:text-foreground flex items-center gap-1.5 text-sm transition-colors"
        >
          <LuArrowLeft className="size-4" />
          Back to Command Line
        </button>
        <div className="ml-auto flex items-center gap-2 text-sm">
          <div className="bg-brand/10 flex size-8 items-center justify-center rounded-md">
            <LuTerminal className="text-brand size-4" />
          </div>
          <div className="hidden flex-col sm:flex">
            <span className="text-foreground font-semibold tracking-wide">
              CLI terminal
            </span>
            <span className="text-foreground-alt text-xs">
              Session {sessionIdx}
            </span>
          </div>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <TerminalPane connectTerminal={connectTerminal} />
      </div>
    </div>
  )
}

async function* runCliTerminal(
  pluginHost: PluginHost,
  instanceKey: string,
  frames: MessageStream<TerminalFrame>,
  signal: AbortSignal,
): MessageStream<TerminalFrame> {
  const loadAbort = new AbortController()
  const lease = holdCliPlugin(pluginHost, instanceKey, loadAbort.signal)
  try {
    await lease.running
    if (signal.aborted) return

    const pluginRpcClient = new SRPCClient(
      buildRpcStreamOpenStream(
        cliTerminalPluginId + '/' + instanceKey,
        pluginHost.PluginRpc,
      ),
    )
    const cliTerminal = new CliTerminalServiceClient(pluginRpcClient)
    for await (const frame of cliTerminal.RunCli(frames, signal)) {
      yield frame
    }
  } catch (err) {
    if (!signal.aborted && !loadAbort.signal.aborted) {
      const message = err instanceof Error ? err.message : String(err)
      yield {
        kind: TerminalFrameKind.ERROR,
        error: message,
      }
    }
  } finally {
    loadAbort.abort()
    void lease.done
  }
}

function holdCliPlugin(
  pluginHost: PluginHost,
  instanceKey: string,
  signal: AbortSignal,
): PluginLoadLease {
  const running = Promise.withResolvers<void>()
  let settled = false
  const settleRunning = () => {
    if (settled) return
    settled = true
    running.resolve()
  }
  const rejectRunning = (err: unknown) => {
    if (settled) return
    settled = true
    running.reject(err)
  }
  const handleAbort = () => {
    rejectRunning(new Error('CLI plugin load was aborted.'))
  }
  signal.addEventListener('abort', handleAbort, { once: true })

  const done = (async () => {
    try {
      for await (const response of pluginHost.LoadPlugin(
        { pluginId: cliTerminalPluginId, instanceKey },
        signal,
      )) {
        if (response.pluginStatus?.running) {
          settleRunning()
        }
      }
      if (!signal.aborted) {
        rejectRunning(new Error('CLI plugin stopped before it became ready.'))
      }
    } catch (err) {
      if (!signal.aborted) {
        rejectRunning(err)
      }
    } finally {
      signal.removeEventListener('abort', handleAbort)
    }
  })()

  return { running: running.promise, done }
}
