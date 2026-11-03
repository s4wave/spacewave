import type { StyleXStyles as BottomBarStyles } from '@stylexjs/stylex'
import { useCallback, useMemo } from 'react'
import { useBldrContext } from '@aptre/bldr-react'
import { Client as SRPCClient } from 'starpc'
import type { MessageStream } from 'starpc'
import { LuTerminal } from '@s4wave/web/images/icons.js'

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
import { useCliTerminalSession } from '@s4wave/app/terminal/CliTerminalSessionProvider.js'
import { SessionFrame } from '@s4wave/app/session/SessionFrame.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'

import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import * as stylex from '@stylexjs/stylex'

import {
  colors as themeColors,
  fonts as themeFonts,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flexShrink: 1,
    flexGrow: 1,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule2: {
    marginInline: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: '1px',
    backgroundColor: themeColors['--color-border'],
  },
  rule3: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule4: {
    display: 'flex',
    minHeight: 0,
    flex: 1,
    flexDirection: 'column',
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background-dark'],
  },
  rule5: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    marginRight: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
})

const cliTerminalServiceId =
  'plugin/spacewave-cli-plugin/' + CliTerminalServiceServiceName

// singleTerminalFrame routes a local connection failure through the same stream
// lifecycle as terminal RPC output.
function singleTerminalFrame(
  frame: TerminalFrame,
): MessageStream<TerminalFrame> {
  return {
    [Symbol.asyncIterator](): AsyncIterator<TerminalFrame> {
      let pendingFrame: TerminalFrame | null = frame
      return {
        next(): Promise<IteratorResult<TerminalFrame>> {
          if (pendingFrame) {
            const value = pendingFrame
            pendingFrame = null
            return Promise.resolve({ done: false, value })
          }
          return Promise.resolve({ done: true, value: undefined })
        },
      }
    },
  }
}

// CliTerminalPage renders the session-local browser CLI terminal in the shell frame.
export function CliTerminalPage() {
  const sessionIndex = useSessionIndex()
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

  const openTerminal = useMemo<TerminalPaneConnector>(() => {
    if (!cliTerminal) {
      return () =>
        singleTerminalFrame({
          kind: TerminalFrameKind.ERROR,
          error: 'runtime-unavailable',
        })
    }
    return (frames, signal) => cliTerminal.RunCli(frames, signal)
  }, [cliTerminal])

  const connectTerminal = useCliTerminalSession(sessionIndex, openTerminal)

  const renderTerminalBarItem = useCallback(
    (selected: boolean, onClick: () => void, sx?: BottomBarStyles) => (
      <BottomBarItem selected={selected} onClick={onClick} sx={sx}>
        <LuTerminal sx={stylexStyles.rule1} />
        <div sx={stylexStyles2.rule1}>Terminal</div>
        <div sx={stylexStyles2.rule2} />
        <div sx={stylexStyles2.rule3}>CLI</div>
      </BottomBarItem>
    ),
    [],
  )

  return (
    <SessionFrame>
      <BottomBarLevel
        id="sessionCliTerminal"
        button={renderTerminalBarItem}
        menuLabel="Terminal"
      >
        <div sx={stylexStyles2.rule4}>
          <div sx={stylexStyles2.rule5}>Spacewave CLI · This session</div>
          <TerminalPane session="cli" connectTerminal={connectTerminal} />
        </div>
      </BottomBarLevel>
    </SessionFrame>
  )
}
