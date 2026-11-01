import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import type { PairingStatus } from '@s4wave/sdk/session/session.pb.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toPairingView } from '../status/pairing.js'

interface PairingLoadingCardProps {
  className?: never
  status?: PairingStatus
  pairingCode?: string
  errorMessage?: string
  onRetry?: () => void
  onCancel?: () => void
  sx?: ComponentStyles
}

// PairingLoadingCard renders a pairing status snapshot as a LoadingCard with
// stage-specific detail text for all 13 PairingStatus substates.
export function PairingLoadingCard({
  status,
  pairingCode,
  errorMessage,
  onRetry,
  onCancel,
  sx,
}: Omit<PairingLoadingCardProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <LoadingCard
      view={toPairingView({
        status,
        pairingCode,
        errorMessage,
        onRetry,
        onCancel,
      })}
      sx={sx}
    />
  )
}
