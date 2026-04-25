import { describe, expect, it } from 'vitest'

import {
  completeAndDismissLocalSessionOnboardingProviderChoice,
  defaultLocalSessionOnboardingState,
} from './local-session-onboarding-state.js'

describe('local-session-onboarding-state', () => {
  it('completes provider choice and dismisses in one transition', () => {
    const next = completeAndDismissLocalSessionOnboardingProviderChoice(
      defaultLocalSessionOnboardingState,
      123,
    )

    expect(next.providerChoiceComplete).toBe(true)
    expect(next.dismissed).toBe(true)
    expect(next.dismissedAt).toBe(123)
    expect(next.backupComplete).toBe(false)
    expect(next.lockComplete).toBe(false)
  })
})
