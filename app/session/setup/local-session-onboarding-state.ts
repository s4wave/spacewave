import superjson from 'superjson'

import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'

export interface LocalSessionOnboardingState {
  dismissed: boolean
  dismissedAt: number | null
  providerChoiceComplete: boolean
  backupComplete: boolean
  lockComplete: boolean
}

type StoredLocalSessionOnboardingState =
  Partial<LocalSessionOnboardingState> & {
    providerComplete?: boolean
  }

export const defaultLocalSessionOnboardingState: LocalSessionOnboardingState = {
  dismissed: false,
  dismissedAt: null,
  providerChoiceComplete: false,
  backupComplete: false,
  lockComplete: false,
}

export const localSessionOnboardingStoreId = 'session/setup/banner'

export function completeLocalSessionOnboardingProviderChoice(
  state: LocalSessionOnboardingState,
): LocalSessionOnboardingState {
  return { ...state, providerChoiceComplete: true }
}

export function completeAndDismissLocalSessionOnboardingProviderChoice(
  state: LocalSessionOnboardingState,
  now = Date.now(),
): LocalSessionOnboardingState {
  return dismissLocalSessionOnboarding(
    completeLocalSessionOnboardingProviderChoice(state),
    now,
  )
}

export function completeLocalSessionOnboardingBackup(
  state: LocalSessionOnboardingState,
): LocalSessionOnboardingState {
  return { ...state, backupComplete: true }
}

export function completeLocalSessionOnboardingLock(
  state: LocalSessionOnboardingState,
): LocalSessionOnboardingState {
  return { ...state, lockComplete: true }
}

export function dismissLocalSessionOnboarding(
  state: LocalSessionOnboardingState,
  now = Date.now(),
): LocalSessionOnboardingState {
  return {
    ...state,
    dismissed: true,
    dismissedAt: now,
  }
}

export function getLocalSessionOnboardingProviderChoiceComplete(
  state: LocalSessionOnboardingState,
  metadata?: SessionMetadata,
): boolean {
  return state.providerChoiceComplete || !!metadata?.cloudAccountId
}

export function isLocalSessionOnboardingComplete(
  state: LocalSessionOnboardingState,
  metadata?: SessionMetadata,
): boolean {
  return (
    getLocalSessionOnboardingProviderChoiceComplete(state, metadata) &&
    state.backupComplete &&
    state.lockComplete
  )
}

export function parseLocalSessionOnboardingState(
  stateJson?: string,
): LocalSessionOnboardingState {
  if (!stateJson) return defaultLocalSessionOnboardingState
  try {
    const parsed = superjson.parse(stateJson)
    if (!parsed || typeof parsed !== 'object') {
      return defaultLocalSessionOnboardingState
    }
    const stored = parsed as StoredLocalSessionOnboardingState
    return {
      ...defaultLocalSessionOnboardingState,
      ...stored,
      providerChoiceComplete:
        stored.providerChoiceComplete ?? stored.providerComplete ?? false,
    }
  } catch {
    return defaultLocalSessionOnboardingState
  }
}
