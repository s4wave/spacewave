import { useCallback, useMemo } from 'react'
import { useBldrContext } from '@aptre/bldr-react'
import { Client as SRPCClient } from 'starpc'
import type { MessageStream } from 'starpc'
import { LuTerminal } from 'react-icons/lu'

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
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'

const cliTerminalServiceId =
  'plugin/spacewave-cli-plugin/' + CliTerminalServiceServiceName

// CliTerminalPage renders the session-local browser CLI terminal in the shell frame.
export function CliTerminalPage() {
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

  const renderTerminalBarItem = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => (
      <BottomBarItem
        selected={selected}
        onClick={onClick}
        className={className}
      >
        <LuTerminal className="mr-1.5 size-3.5 shrink-0" />
        <div className="flex-shrink flex-grow truncate">Terminal</div>
        <div className="bg-border mx-2 h-3 w-px" />
        <div className="text-muted-foreground truncate text-xs">CLI</div>
      </BottomBarItem>
    ),
    [],
  )

  return (
    <SessionFrame>
      <BottomBarLevel
        id="sessionCliTerminal"
        button={renderTerminalBarItem}
        buttonKey="session-cli-terminal"
        menuLabel="Terminal"
      >
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-zinc-950">
          <TerminalPane connectTerminal={connectTerminal} />
        </div>
      </BottomBarLevel>
    </SessionFrame>
  )
}
