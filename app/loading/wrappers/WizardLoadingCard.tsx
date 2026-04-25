import type { WizardState } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { toWizardView } from '../status/wizard.js'

interface WizardLoadingCardProps {
  state: WizardState | null | undefined
  loading: boolean
  errorMessage?: string
  totalSteps?: number
  activeStepLabel?: string
  onRetry?: () => void
  onCancel?: () => void
  className?: string
}

// WizardLoadingCard renders a wizard handle state as a LoadingCard with step
// N of M + active-step detail. Use for any wizard viewer that wants a
// consistent "loading wizard" surface.
export function WizardLoadingCard({
  state,
  loading,
  errorMessage,
  totalSteps,
  activeStepLabel,
  onRetry,
  onCancel,
  className,
}: WizardLoadingCardProps) {
  return (
    <LoadingCard
      view={toWizardView({
        state,
        loading,
        errorMessage,
        totalSteps,
        activeStepLabel,
        onRetry,
        onCancel,
      })}
      className={className}
    />
  )
}
