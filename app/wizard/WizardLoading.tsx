import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { WizardGone } from './WizardGone.js'

/** WizardLoadingProps names the wait a wizard shows before its state arrives. */
export interface WizardLoadingProps {
  // wizardGone is the useWizardState watch on the wizard object.
  wizardGone: boolean
  title?: string
  detail: string
}

/**
 * WizardLoading is what every wizard viewer shows until its state arrives: a
 * loading card while the wizard object exists, or the gone state once it is
 * removed, since a removed wizard never streams a state.
 */
export function WizardLoading({
  wizardGone,
  title = 'Loading wizard',
  detail,
}: WizardLoadingProps) {
  if (wizardGone) return <WizardGone />
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard view={{ state: 'active', title, detail }} />
      </div>
    </div>
  )
}
