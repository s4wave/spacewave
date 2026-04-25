import { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import type { LoadingState, LoadingView } from '@s4wave/web/ui/loading/types.js'

interface PairingViewInput {
  status?: PairingStatus
  pairingCode?: string
  errorMessage?: string
  onRetry?: () => void
  onCancel?: () => void
}

// toPairingView maps all 13 PairingStatus substates into a LoadingView with
// stage-specific detail text. Error substates (FAILED, SIGNALING_FAILED,
// CONNECTION_TIMEOUT, PAIRING_REJECTED, CONFIRMATION_TIMEOUT) flip to
// 'error'. VERIFIED / BOTH_CONFIRMED flip to 'synced'.
export function toPairingView(input: PairingViewInput): LoadingView {
  const { status, pairingCode, errorMessage, onRetry, onCancel } = input
  const stage = pairingStage(status ?? PairingStatus.PairingStatus_IDLE)
  if (stage.state === 'error') {
    return {
      state: stage.state,
      title: stage.title,
      detail: stage.detail,
      error: errorMessage || undefined,
      onRetry,
      onCancel,
    }
  }
  const detail =
    stage.state === 'active' && pairingCode ?
      `${stage.detail} Code: ${pairingCode}`
    : stage.detail
  return {
    state: stage.state,
    title: stage.title,
    detail,
    onCancel: stage.state === 'active' ? onCancel : undefined,
  }
}

interface PairingStage {
  state: LoadingState
  title: string
  detail: string
}

function pairingStage(status: PairingStatus): PairingStage {
  switch (status) {
    case PairingStatus.PairingStatus_IDLE:
      return {
        state: 'loading',
        title: 'Preparing pairing',
        detail: 'Setting up the pairing channel.',
      }
    case PairingStatus.PairingStatus_CODE_GENERATED:
      return {
        state: 'active',
        title: 'Pairing code ready',
        detail: 'Enter the code on the other device.',
      }
    case PairingStatus.PairingStatus_WAITING_FOR_PEER:
      return {
        state: 'active',
        title: 'Waiting for peer',
        detail: 'Other device is connecting.',
      }
    case PairingStatus.PairingStatus_PEER_CONNECTED:
      return {
        state: 'active',
        title: 'Peer connected',
        detail: 'Secure channel established. Exchanging verification data.',
      }
    case PairingStatus.PairingStatus_VERIFYING_EMOJI:
      return {
        state: 'active',
        title: 'Verify the emoji sequence',
        detail: 'Confirm the same emoji appear on both devices.',
      }
    case PairingStatus.PairingStatus_WAITING_FOR_REMOTE_CONFIRM:
      return {
        state: 'active',
        title: 'Waiting for remote confirmation',
        detail: 'The other device still needs to confirm verification.',
      }
    case PairingStatus.PairingStatus_BOTH_CONFIRMED:
      return {
        state: 'synced',
        title: 'Verification confirmed',
        detail: 'Both devices confirmed. Finishing pairing.',
      }
    case PairingStatus.PairingStatus_VERIFIED:
      return {
        state: 'synced',
        title: 'Paired',
        detail: 'Devices are linked.',
      }
    case PairingStatus.PairingStatus_FAILED:
      return {
        state: 'error',
        title: 'Pairing failed',
        detail: 'The pairing attempt could not complete.',
      }
    case PairingStatus.PairingStatus_SIGNALING_FAILED:
      return {
        state: 'error',
        title: 'Signaling failed',
        detail: 'Unable to reach the signaling service to set up the pairing.',
      }
    case PairingStatus.PairingStatus_CONNECTION_TIMEOUT:
      return {
        state: 'error',
        title: 'Connection timed out',
        detail: 'The other device did not respond in time.',
      }
    case PairingStatus.PairingStatus_PAIRING_REJECTED:
      return {
        state: 'error',
        title: 'Pairing rejected',
        detail: 'The other device rejected the pairing request.',
      }
    case PairingStatus.PairingStatus_CONFIRMATION_TIMEOUT:
      return {
        state: 'error',
        title: 'Confirmation timed out',
        detail: 'Emoji verification was not confirmed in time.',
      }
    default:
      return {
        state: 'loading',
        title: 'Pairing',
        detail: 'Starting pairing flow.',
      }
  }
}
