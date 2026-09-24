import { useCallback, useEffect, useRef, useState } from 'react'
import { LuBot } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { usePath } from '@s4wave/web/router/router.js'
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from '@s4wave/web/ui/Popover.js'
import { useListenerStatus } from '@s4wave/app/hooks/useListenerStatus.js'

import { pairingStatusReachedPeer } from '../pairing-status.js'
import {
  buildAgentPrompt,
  parseAgentContext,
  type AgentAccess,
  type AgentContext,
} from './agent-prompt.js'
import {
  AgentConnectPanel,
  type AgentPairing,
  type CopyState,
} from './AgentConnectPanel.js'

// AGENT_CONNECT_MENU_ID is the bottom-bar level ID of the connect panel.
const AGENT_CONNECT_MENU_ID = 'agent-connect'

// CODE_TTL_MS is the pairing relay's code lifetime.
const CODE_TTL_MS = 10 * 60 * 1000

// AgentConnectButton registers the Connect an agent bottom-bar item. Opening
// it copies a prompt carrying a fresh pairing code (or the desktop CLI socket)
// and the current Space, then shows the approval panel. The pairing attempt
// lives here so the panel reopens when the agent connects while it is closed.
export function AgentConnectButton() {
  const session = useResourceValue(SessionContext.useContext())
  const { providerId } = useSessionInfo(session)
  const listener = useListenerStatus()
  const routePath = usePath()
  const setOpenMenu = useBottomBarSetOpenMenu()

  const socketPath = listener?.listening ? listener.socketPath : ''
  const pairingSupported = providerId === 'local' || providerId === 'spacewave'

  const [pairing, setPairing] = useState<AgentPairing | null>(null)
  const [copyState, setCopyState] = useState<CopyState>('idle')
  const attemptRef = useRef<AbortController | null>(null)

  useEffect(() => () => attemptRef.current?.abort(), [])

  // copyPrompt writes the prompt inside the click's user activation. The
  // ClipboardItem takes a promise so the code may arrive after the gesture.
  const copyPrompt = useCallback(
    (access: Promise<AgentAccess>) => {
      if (!session) return
      const context = parseAgentContext(routePath)
      const text = Promise.all([access, readSpaceName(session, context)]).then(
        ([resolved, spaceName]) =>
          buildAgentPrompt(resolved, { ...context, spaceName }),
      )
      const write =
        typeof ClipboardItem === 'undefined'
          ? text.then((value) => navigator.clipboard.writeText(value))
          : navigator.clipboard.write([
              new ClipboardItem({
                'text/plain': text.then(
                  (value) => new Blob([value], { type: 'text/plain' }),
                ),
              }),
            ])
      setCopyState('copying')
      write.then(
        () => setCopyState('copied'),
        () => setCopyState('failed'),
      )
    },
    [session, routePath],
  )

  // startPairing registers a new pairing code and copies its prompt.
  const startPairing = useCallback(() => {
    if (!session) return
    attemptRef.current?.abort()
    const controller = new AbortController()
    attemptRef.current = controller
    setPairing({ code: '', expiresAt: 0, remotePeerId: '', error: '' })
    const code = session.generatePairingCode(controller.signal)
    code.then(
      (value) => {
        if (controller.signal.aborted) return
        setPairing({
          code: value,
          expiresAt: Date.now() + CODE_TTL_MS,
          remotePeerId: '',
          error: '',
        })
      },
      (cause: unknown) => {
        if (controller.signal.aborted) return
        setPairing({
          code: '',
          expiresAt: 0,
          remotePeerId: '',
          error:
            cause instanceof Error ? cause.message : 'Could not create a code',
        })
      },
    )
    copyPrompt(
      code.then((value) => ({
        kind: 'code',
        code: value,
        ttlMinutes: CODE_TTL_MS / 60000,
      })),
    )
  }, [session, copyPrompt])

  // copyAgain copies the prompt for the current access without a new code.
  const copyAgain = useCallback(() => {
    if (socketPath) {
      copyPrompt(Promise.resolve({ kind: 'socket', socketPath }))
      return
    }
    if (!pairing?.code || pairing.expiresAt <= Date.now()) {
      startPairing()
      return
    }
    copyPrompt(
      Promise.resolve({
        kind: 'code',
        code: pairing.code,
        ttlMinutes: Math.ceil((pairing.expiresAt - Date.now()) / 60000),
      }),
    )
  }, [socketPath, pairing, copyPrompt, startPairing])

  // Watch the attempt until the agent's CLI connects, then open the panel.
  const code = pairing?.code ?? ''
  const waiting = !!code && !pairing?.remotePeerId
  useEffect(() => {
    if (!session || !waiting) return
    const controller = new AbortController()
    // update applies a result to this attempt unless it was replaced.
    const update = (next: Partial<AgentPairing>) => {
      if (controller.signal.aborted) return
      setPairing((prev: AgentPairing | null) =>
        prev?.code === code ? { ...prev, ...next } : prev,
      )
    }
    void (async () => {
      for await (const resp of session.watchPairingStatus(controller.signal)) {
        if (pairingStatusReachedPeer(resp.status) && resp.remotePeerId) {
          update({ remotePeerId: resp.remotePeerId })
          setOpenMenu?.(AGENT_CONNECT_MENU_ID)
          return
        }
        if (
          resp.status === PairingStatus.PairingStatus_SIGNALING_FAILED ||
          resp.status === PairingStatus.PairingStatus_FAILED
        ) {
          update({ error: resp.errorMessage || 'Pairing failed' })
          return
        }
      }
    })().catch((cause: unknown) => {
      update({
        error: cause instanceof Error ? cause.message : 'Pairing status failed',
      })
    })
    return () => controller.abort()
  }, [session, waiting, code, setOpenMenu])

  const buttonRender = useCallback(
    (selected: boolean, onClick: () => void, className?: string) => {
      // Opening copies the prompt unless an agent is mid-approval.
      const handleClick = () => {
        if (!selected && !pairing?.remotePeerId) {
          copyAgain()
        }
        onClick()
      }
      return (
        <Popover open={selected}>
          <PopoverAnchor asChild>
            <BottomBarItem
              selected={selected}
              onClick={handleClick}
              className={className}
              aria-label={
                selected ? 'Close agent connection' : 'Connect an agent'
              }
              data-testid="agent-connect-button"
            >
              <LuBot aria-hidden="true" />
            </BottomBarItem>
          </PopoverAnchor>
          <PopoverContent
            side="top"
            align="end"
            sideOffset={6}
            onEscapeKeyDown={onClick}
            onInteractOutside={(event) => event.preventDefault()}
            variant="status"
          >
            <AgentConnectPanel
              session={session}
              socketPath={socketPath}
              connectedClients={listener?.connectedClients ?? 0}
              pairing={pairing}
              copyState={copyState}
              onCopy={copyAgain}
              onNewCode={startPairing}
              onClose={onClick}
            />
          </PopoverContent>
        </Popover>
      )
    },
    [
      session,
      socketPath,
      listener?.connectedClients,
      pairing,
      copyState,
      copyAgain,
      startPairing,
    ],
  )

  if (!session || (!socketPath && !pairingSupported)) {
    return null
  }

  // The bottom bar re-renders the button only when this key changes, so it
  // names every value the panel and its callbacks read.
  const buttonKey = [
    routePath,
    socketPath,
    listener?.connectedClients ?? 0,
    pairing?.code ?? '',
    pairing?.remotePeerId ?? '',
    pairing?.error ?? '',
    copyState,
  ].join('|')

  return (
    <BottomBarLevel
      id={AGENT_CONNECT_MENU_ID}
      position="right"
      button={buttonRender}
      buttonKey={buttonKey}
    >
      {null}
    </BottomBarLevel>
  )
}

// readSpaceName reads the display name of the context's Space from the first
// Space list snapshot, or returns undefined when it is not listed.
async function readSpaceName(
  session: Session,
  context: AgentContext,
): Promise<string | undefined> {
  if (!context.spaceId) return undefined
  const controller = new AbortController()
  try {
    for await (const resp of session.watchResourcesList(
      {},
      controller.signal,
    )) {
      const entry = resp.spacesList?.find(
        (space) =>
          space.entry?.ref?.providerResourceRef?.id === context.spaceId,
      )
      return entry?.spaceMeta?.name || undefined
    }
  } catch {
    // The name is optional; the prompt carries the Space ID regardless.
  } finally {
    controller.abort()
  }
  return undefined
}
