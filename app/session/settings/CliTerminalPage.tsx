import { useCallback, useMemo } from 'react'
import { useBldrContext } from '@aptre/bldr-react'
import { Client as SRPCClient } from 'starpc'
import type { MessageStream } from 'starpc'
import { LuArrowLeft, LuTerminal } from 'react-icons/lu'

import {
  CliTerminalServiceClient,
  CliTerminalServiceServiceName,
} from '@s4wave/sdk/cli/terminal/terminal_srpc.pb.js'
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

const cliTerminalServiceId =
  'plugin/spacewave-cli-plugin/' + CliTerminalServiceServiceName

// CliTerminalPage renders the session-local in-app CLI terminal.
export function CliTerminalPage() {
  const navigate = useNavigate()
  const sessionIdx = useSessionIndex()
  const bldrContext = useBldrContext()
  const webDocument = bldrContext?.webDocument ?? null
  const webViewUuid = bldrContext?.webView?.getUuid() ?? null

  const cliTerminal = useMemo(() => {
    if (!webDocument || !webViewUuid) return null
    const rpcClient = new SRPCClient(
      webDocument.buildWebViewHostOpenStream(webViewUuid),
    )
    return new CliTerminalServiceClient(rpcClient, {
      service: cliTerminalServiceId,
    })
  }, [webDocument, webViewUuid])

  const connectTerminal = useMemo<TerminalPaneConnector>(() => {
    if (!cliTerminal) {
      return async function* (): MessageStream<TerminalFrame> {
        yield {
          kind: TerminalFrameKind.ERROR,
          error: 'Spacewave runtime context is unavailable.',
        }
      }
    }
    return (frames, signal) => cliTerminal.RunCli(frames, signal)
  }, [cliTerminal])

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
