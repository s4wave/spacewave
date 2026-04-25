import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react'

import type { SessionMetadata } from '@s4wave/core/session/session.pb.js'
import {
  useBackendStateAtomValue,
  useStateNamespace,
} from '@s4wave/web/state/index.js'
import type { StateAtomAccessor } from '@s4wave/web/state/index.js'

import {
  completeLocalSessionOnboardingBackup,
  completeLocalSessionOnboardingLock,
  completeLocalSessionOnboardingProviderChoice,
  defaultLocalSessionOnboardingState,
  dismissLocalSessionOnboarding,
  getLocalSessionOnboardingProviderChoiceComplete,
  isLocalSessionOnboardingComplete,
  type LocalSessionOnboardingState,
} from './local-session-onboarding-state.js'

interface LocalSessionOnboardingContextValue {
  onboarding: LocalSessionOnboardingState
  loading: boolean
  metadataLoaded: boolean
  providerChoiceComplete: boolean
  isComplete: boolean
  setOnboarding: (
    update:
      | LocalSessionOnboardingState
      | ((prev: LocalSessionOnboardingState) => LocalSessionOnboardingState),
  ) => void
  markProviderChoiceComplete: () => void
  markBackupComplete: () => void
  markLockComplete: () => void
  dismiss: () => void
}

const Context = createContext<LocalSessionOnboardingContextValue | null>(null)

const nullStateAtomAccessor: StateAtomAccessor = {
  value: null,
  loading: false,
  error: null,
  retry: () => {},
}

export function useSessionOnboardingState(
  metadata?: SessionMetadata,
): LocalSessionOnboardingContextValue {
  const setupNs = useStateNamespace(['setup'])
  const storeId = useMemo(
    () => [...setupNs.namespace, 'banner'].join('/'),
    [setupNs.namespace],
  )
  const {
    value: onboarding,
    loading,
    setValue: setBackendOnboarding,
  } = useBackendStateAtomValue<LocalSessionOnboardingState>(
    setupNs.stateAtomAccessor ?? nullStateAtomAccessor,
    storeId,
    defaultLocalSessionOnboardingState,
  )

  const setOnboarding = useCallback(
    (
      update:
        | LocalSessionOnboardingState
        | ((prev: LocalSessionOnboardingState) => LocalSessionOnboardingState),
    ) => {
      if (loading) return
      setBackendOnboarding(update)
    },
    [loading, setBackendOnboarding],
  )

  const markProviderChoiceComplete = useCallback(() => {
    setOnboarding(completeLocalSessionOnboardingProviderChoice)
  }, [setOnboarding])

  const markBackupComplete = useCallback(() => {
    setOnboarding(completeLocalSessionOnboardingBackup)
  }, [setOnboarding])

  const markLockComplete = useCallback(() => {
    setOnboarding(completeLocalSessionOnboardingLock)
  }, [setOnboarding])

  const dismiss = useCallback(() => {
    setOnboarding(dismissLocalSessionOnboarding)
  }, [setOnboarding])

  const providerChoiceComplete = useMemo(
    () => getLocalSessionOnboardingProviderChoiceComplete(onboarding, metadata),
    [onboarding, metadata],
  )
  const metadataLoaded = metadata !== undefined
  const isComplete = useMemo(
    () => isLocalSessionOnboardingComplete(onboarding, metadata),
    [onboarding, metadata],
  )

  return useMemo(
    () => ({
      onboarding,
      loading,
      metadataLoaded,
      providerChoiceComplete,
      isComplete,
      setOnboarding,
      markProviderChoiceComplete,
      markBackupComplete,
      markLockComplete,
      dismiss,
    }),
    [
      onboarding,
      loading,
      metadataLoaded,
      providerChoiceComplete,
      isComplete,
      setOnboarding,
      markProviderChoiceComplete,
      markBackupComplete,
      markLockComplete,
      dismiss,
    ],
  )
}

export function LocalSessionOnboardingProvider({
  metadata,
  children,
}: {
  metadata?: SessionMetadata
  children?: ReactNode
}) {
  const value = useSessionOnboardingState(metadata)

  return <Context.Provider value={value}>{children}</Context.Provider>
}

export function useLocalSessionOnboardingContext(): LocalSessionOnboardingContextValue {
  const context = useContext(Context)
  if (!context) {
    throw new Error(
      'LocalSessionOnboardingContext not found. Wrap component in LocalSessionOnboardingProvider.',
    )
  }
  return context
}

export function useOptionalLocalSessionOnboardingContext(): LocalSessionOnboardingContextValue | null {
  return useContext(Context)
}
