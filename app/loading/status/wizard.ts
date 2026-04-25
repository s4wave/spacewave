import type { WizardState } from '@s4wave/sdk/world/wizard/wizard.pb.js'
import type { LoadingView } from '@s4wave/web/ui/loading/types.js'

interface WizardViewInput {
  state: WizardState | null | undefined
  loading: boolean
  errorMessage?: string
  totalSteps?: number
  activeStepLabel?: string
  onRetry?: () => void
  onCancel?: () => void
}

// toWizardView maps a generic WizardHandle state into a LoadingView. Wizards
// rarely have determinate progress, so the view defaults to an active step
// label with step N of M when total steps are known.
export function toWizardView(input: WizardViewInput): LoadingView {
  const {
    state,
    loading,
    errorMessage,
    totalSteps,
    activeStepLabel,
    onRetry,
    onCancel,
  } = input
  if (errorMessage) {
    return {
      state: 'error',
      title: 'Wizard failed',
      detail: activeStepLabel ?? 'The wizard could not complete.',
      error: errorMessage,
      onRetry,
      onCancel,
    }
  }
  if (loading && !state) {
    return {
      state: 'loading',
      title: 'Loading wizard',
      detail: 'Preparing the wizard state.',
      onCancel,
    }
  }
  const step = state?.step ?? 0
  const stepLabel =
    totalSteps !== undefined ?
      `Step ${step + 1} of ${totalSteps}`
    : `Step ${step + 1}`
  const title =
    state?.name ? `Configuring ${state.name}`
    : state?.targetTypeId ? `Configuring ${state.targetTypeId}`
    : 'Wizard'
  return {
    state: 'active',
    title,
    detail: activeStepLabel ? `${stepLabel}: ${activeStepLabel}` : stepLabel,
    onCancel,
  }
}
