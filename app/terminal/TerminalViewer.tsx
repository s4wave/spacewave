import { useCallback, useMemo } from 'react'
import { LuTerminal } from '@s4wave/web/images/icons.js'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import {
  TerminalSessionState,
  TerminalTargetKind,
  type Terminal,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import {
  TerminalHandle,
  TerminalTypeID,
} from '@s4wave/sdk/terminal/terminal.js'
import {
  TerminalPane,
  safeTerminalFailureDetail,
  type TerminalPaneConnector,
  type TerminalPaneTrustChallengeRenderer,
} from './TerminalPane.js'
import { TerminalSshTrustPanel } from './TerminalSshTrustPanel.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  rule5: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    minWidth: 0,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-muted-foreground'],
  },
  rule7: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    color: themeColors['--color-foreground'],
  },
  rule8: { color: themeColors['--color-foreground'] },
  rule9: { padding: `calc(${themeLayout['--spacing']} * 4)` },
  rule10: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
})

export { TerminalTypeID }

export function TerminalViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)

  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    TerminalHandle,
    TerminalTypeID,
  )
  const terminalHandle = handle.value

  const streamFactory = useCallback(
    (h: TerminalHandle, signal: AbortSignal) => h.watchTerminalState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Terminal | undefined = stateResource.value ?? undefined
  useObjectName(state?.name)

  const connectTerminal = useMemo<TerminalPaneConnector | undefined>(() => {
    if (!terminalHandle) return undefined
    return (frames, signal) => terminalHandle.connectTerminal(frames, signal)
  }, [terminalHandle])

  const renderTrustChallenge = useCallback<TerminalPaneTrustChallengeRenderer>(
    (challenge, onRespond) => {
      return (
        <TerminalSshTrustPanel challenge={challenge} onRespond={onRespond} />
      )
    },
    [],
  )

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <span sx={stylexStyles2.rule3}>
          <LuTerminal sx={stylexStyles.rule1} />
          {state?.name || 'Terminal'}
        </span>
        <span sx={stylexStyles2.rule4}>
          {formatTerminalState(state?.state)}
        </span>
      </div>
      <div sx={stylexStyles2.rule5}>
        <span sx={stylexStyles2.rule6}>
          {formatTerminalTargetLabel(state)}{' '}
          <span sx={stylexStyles2.rule7}>
            {formatTerminalTargetObjectKey(state, objectKey)}
          </span>
        </span>
        {state?.command && (
          <span sx={stylexStyles2.rule6}>
            command <span sx={stylexStyles2.rule8}>{state.command}</span>
          </span>
        )}
      </div>
      {stateResource.loading && !state && (
        <div sx={stylexStyles2.rule9}>
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading terminal',
              detail: 'Reading terminal state.',
            }}
          />
        </div>
      )}
      {state?.error && !connectTerminal && (
        <div sx={stylexStyles2.rule10}>
          {safeTerminalFailureDetail(state.error)}
        </div>
      )}
      <TerminalPane
        session="terminal"
        connectTerminal={connectTerminal}
        renderTrustChallenge={renderTrustChallenge}
      />
    </div>
  )
}

function formatTerminalTargetLabel(state?: Terminal): string {
  return state?.targetKind === TerminalTargetKind.SSH_HOST
    ? 'ssh host'
    : 'device'
}

function formatTerminalTargetObjectKey(
  state: Terminal | undefined,
  fallback: string,
): string {
  if (state?.targetKind === TerminalTargetKind.SSH_HOST) {
    return state.sshHostObjectKey || fallback
  }
  return state?.deviceObjectKey || fallback
}

function formatTerminalState(state?: TerminalSessionState): string {
  switch (state) {
    case TerminalSessionState.CONNECTING:
      return 'Connecting'
    case TerminalSessionState.ACTIVE:
      return 'Active'
    case TerminalSessionState.FAILED:
      return 'Failed'
    case TerminalSessionState.CLOSED:
      return 'Closed'
    case TerminalSessionState.DISCONNECTED:
      return 'Disconnected'
    default:
      return 'Idle'
  }
}
