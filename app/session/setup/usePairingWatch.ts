import { useEffect } from 'react'

import { pairingStatusReachedPeer } from '@s4wave/app/session/pairing-status.js'
import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'

// PairingOutcome is how a pairing status stream ended: a connected peer, or a
// transport failure.
type PairingOutcome =
  | { kind: 'peer'; peerId: string }
  | { kind: 'failure'; message: string }

// awaitPairingOutcome reads the pairing status stream until a peer connects.
// With reportFailures it also ends on a signaling or pairing failure. It
// returns null when the stream ends first.
async function awaitPairingOutcome(
  session: Session,
  signal: AbortSignal,
  reportFailures: boolean,
): Promise<PairingOutcome | null> {
  for await (const resp of session.watchPairingStatus(signal)) {
    if (signal.aborted) return null
    if (pairingStatusReachedPeer(resp.status) && resp.remotePeerId) {
      return { kind: 'peer', peerId: resp.remotePeerId }
    }
    if (!reportFailures) continue
    if (resp.status === PairingStatus.PairingStatus_SIGNALING_FAILED) {
      return {
        kind: 'failure',
        message: resp.errorMessage || 'Signaling connection failed',
      }
    }
    if (resp.status === PairingStatus.PairingStatus_FAILED) {
      return { kind: 'failure', message: resp.errorMessage || 'Pairing failed' }
    }
  }
  return null
}

export interface PairingWatchOptions {
  session: Session | null | undefined
  // enabled gates the watch, which runs only once the local side is waiting
  // for the remote device.
  enabled: boolean
  // reportFailures surfaces signaling and pairing failures through onFailure.
  reportFailures: boolean
  // streamFailure is the onFailure message when the status stream itself fails.
  streamFailure: string
  onPeerResolved: (peerId: string) => void
  onFailure: (message: string) => void
}

// usePairingWatch watches the pairing status while enabled and reports the
// connected peer or the failure.
export function usePairingWatch({
  session,
  enabled,
  reportFailures,
  streamFailure,
  onPeerResolved,
  onFailure,
}: PairingWatchOptions) {
  useEffect(() => {
    if (!session || !enabled) return
    const controller = new AbortController()
    awaitPairingOutcome(session, controller.signal, reportFailures)
      .then((outcome) => {
        if (controller.signal.aborted || !outcome) return
        if (outcome.kind === 'peer') {
          onPeerResolved(outcome.peerId)
          return
        }
        onFailure(outcome.message)
      })
      .catch((err) => {
        if (controller.signal.aborted) return
        onFailure(err instanceof Error ? err.message : streamFailure)
      })
    return () => controller.abort()
  }, [
    session,
    enabled,
    reportFailures,
    streamFailure,
    onPeerResolved,
    onFailure,
  ])
}
