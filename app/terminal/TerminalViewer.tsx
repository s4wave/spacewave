import { useCallback, useMemo } from 'react'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'

import type { Terminal } from '@s4wave/sdk/terminal/terminal.pb.js'
import {
  TerminalHandle,
  TerminalTypeID,
} from '@s4wave/sdk/terminal/terminal.js'
import {
  TerminalPane,
  type TerminalPaneConnector,
  type TerminalPaneTrustChallengeRenderer,
} from './TerminalPane.js'
import { TerminalSshTrustPanel } from './TerminalSshTrustPanel.js'
import { TerminalView, type TerminalViewState } from './TerminalView.js'

export { TerminalTypeID }

// TerminalViewer shows a Terminal object: its session state in the bar and
// its screen connected through the object handle.
export function TerminalViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  // Resolve the object key the handle and the fallback target read.
  const objectKey = getObjectKey(objectInfo)

  // Open the typed handle the screen and the state watch both use.
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    TerminalHandle,
    TerminalTypeID,
  )
  const terminalHandle = handle.value

  // Watch the session state for the bar, Details and the pane title.
  const streamFactory = useCallback(
    (h: TerminalHandle, signal: AbortSignal) => h.watchTerminalState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: Terminal | undefined = stateResource.value ?? undefined
  useObjectName(state?.name)

  // Connect the screen through the handle once it opens.
  const connectTerminal = useMemo<TerminalPaneConnector | undefined>(() => {
    if (!terminalHandle) return undefined
    return (frames, signal) => terminalHandle.connectTerminal(frames, signal)
  }, [terminalHandle])

  // Ask about an unknown SSH host key in the trust panel.
  const renderTrustChallenge = useCallback<TerminalPaneTrustChallengeRenderer>(
    (challenge, onRespond) => {
      return (
        <TerminalSshTrustPanel challenge={challenge} onRespond={onRespond} />
      )
    },
    [],
  )

  // Draw one state derived from the handle and the state watch.
  const view = terminalViewState(handle, stateResource)
  return (
    <TerminalView view={view}>
      <TerminalPane
        session="terminal"
        connectTerminal={connectTerminal}
        renderTrustChallenge={renderTrustChallenge}
      />
    </TerminalView>
  )
}

// terminalViewState derives what the viewer draws, as TerminalViewer.tla
// models it. The handle's status takes the body until it opens. The watch
// reports its parent's failure as its own, so it is read only on an open
// handle, where its failure is a notice above the screen with its own Retry.
// The screen's connection draws its own card inside TerminalPane.
export function terminalViewState(
  handle: Resource<TerminalHandle>,
  watch: Resource<Terminal | undefined>,
): TerminalViewState {
  if (handle.error) {
    return {
      body: 'status',
      status: {
        state: 'failed',
        message: `Could not open the terminal: ${handle.error.message}`,
        retry: handle.retry,
      },
    }
  }
  if (!handle.value) {
    return {
      body: 'status',
      status: { state: 'waiting', what: 'the terminal', rows: 0 },
    }
  }
  if (watch.error) {
    return {
      body: 'screen',
      notice: {
        state: 'failed',
        message: `Could not read the session state: ${watch.error.message}`,
        retry: watch.retry,
      },
    }
  }
  return { body: 'screen', session: watch.value ?? undefined }
}
