import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import { WizardGone } from './WizardGone.js'

/**
 * WizardLoadingProps describes the wait a wizard shows before its state arrives.
 */
export interface WizardLoadingProps {
  // wizardKey is the wizard object's key.
  wizardKey: string
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
  wizardKey,
  wizardGone,
  title = 'Loading wizard',
  detail,
}: WizardLoadingProps) {
  if (wizardGone) return <WizardGone wizardKey={wizardKey} />
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <LoadingCard view={{ state: 'active', title, detail }} />
      </div>
    </div>
  )
}
