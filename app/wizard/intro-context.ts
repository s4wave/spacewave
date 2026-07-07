import { createContext, use } from 'react'

// IntroWizardPresentationContext is true while the new-user introduction wizard
// is presenting the target object beneath its overlay. The introduced object's
// own first-run onboarding chrome reads this to stand down, so the wizard is the
// single first-run guidance surface for that state instead of stacking its own
// callouts on top of the target's separate welcome affordances.
export const IntroWizardPresentationContext = createContext(false)

// useIntroWizardPresentation reports whether an intro wizard is presenting the
// surrounding object viewer.
export function useIntroWizardPresentation(): boolean {
  return use(IntroWizardPresentationContext)
}
