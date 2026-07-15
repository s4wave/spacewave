import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'

// pairingStatusReachedPeer reports whether the pairing flow has reached the
// peer-connected stage or a later non-error stage. The stream may begin after
// the transient PEER_CONNECTED snapshot has already been emitted.
export function pairingStatusReachedPeer(status?: PairingStatus): boolean {
  switch (status) {
    case PairingStatus.PairingStatus_PEER_CONNECTED:
    case PairingStatus.PairingStatus_VERIFYING_EMOJI:
    case PairingStatus.PairingStatus_WAITING_FOR_REMOTE_CONFIRM:
    case PairingStatus.PairingStatus_BOTH_CONFIRMED:
    case PairingStatus.PairingStatus_VERIFIED:
      return true
    default:
      return false
  }
}
