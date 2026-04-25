import { useCallback } from 'react'

import { useWatchStateRpc } from '@aptre/bldr-react'

import {
  WatchListenerStatusRequest,
  WatchListenerStatusResponse,
} from '@s4wave/sdk/root/root.pb.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'

// ListenerStatus describes the current desktop resource listener
// state used to render the command-line setup page's status chip.
export interface ListenerStatus {
  // socketPath is the effective absolute socket path. Empty string
  // means the listener controller has not resolved its configured
  // path yet (e.g. the browser/WASM runtime where the controller is
  // a no-op).
  socketPath: string
  // listening is true when the listener is currently bound and
  // accepting connections.
  listening: boolean
  // connectedClients is the count of clients currently connected.
  connectedClients: number
}

const emptyReq: WatchListenerStatusRequest = {}

// useListenerStatus streams the desktop resource listener status.
// Returns null until the first emission. The underlying eq-fn uses
// the generated WatchListenerStatusResponse equality so the page is
// only re-rendered on socket-path, listening, or client-count
// changes.
export function useListenerStatus(): ListenerStatus | null {
  const root = useRootResource().value
  const watchFn = useCallback(
    (_: WatchListenerStatusRequest, signal: AbortSignal) => {
      if (!root) return null
      return root.watchListenerStatus(emptyReq, signal)
    },
    [root],
  )
  const resp = useWatchStateRpc(
    watchFn,
    emptyReq,
    WatchListenerStatusRequest.equals,
    WatchListenerStatusResponse.equals,
  )
  if (resp == null) return null
  return {
    socketPath: resp.socketPath ?? '',
    listening: resp.listening ?? false,
    connectedClients: resp.connectedClients ?? 0,
  }
}
