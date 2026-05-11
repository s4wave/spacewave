import { useEffect } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import {
  useSessionOnboardingState,
  type LocalSessionOnboardingContextValue,
} from './LocalSessionOnboardingContext.js'
import { defaultLocalSessionOnboardingState } from './local-session-onboarding-state.js'

const mockUseBackendStateAtomValue = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/state/index.js', () => ({
  useStateNamespace: () => ({
    namespace: ['setup'],
    stateAtomAccessor: { value: null, loading: false, error: null },
  }),
  useBackendStateAtomValue: mockUseBackendStateAtomValue,
}))

function BackupProbe() {
  const { loading, markProviderChoiceComplete, markBackupComplete } =
    useSessionOnboardingState()

  useEffect(() => {
    if (!loading) return
    markProviderChoiceComplete()
  }, [loading, markProviderChoiceComplete])

  return (
    <button onClick={markBackupComplete} type="button">
      Save backup
    </button>
  )
}

describe('LocalSessionOnboardingContext', () => {
  afterEach(() => {
    cleanup()
    mockUseBackendStateAtomValue.mockReset()
  })

  it('flushes onboarding mutations queued while backend state is loading', async () => {
    const setValue =
      vi.fn<LocalSessionOnboardingContextValue['setOnboarding']>()
    const state = { loading: true }
    mockUseBackendStateAtomValue.mockImplementation(() => ({
      value: defaultLocalSessionOnboardingState,
      loading: state.loading,
      setValue,
    }))

    const { rerender } = render(<BackupProbe />)

    fireEvent.click(screen.getByRole('button', { name: 'Save backup' }))
    expect(setValue).not.toHaveBeenCalled()

    state.loading = false
    rerender(<BackupProbe />)

    await waitFor(() => expect(setValue).toHaveBeenCalledTimes(1))

    const update = setValue.mock.calls[0]?.[0]
    expect(typeof update).toBe('function')
    if (typeof update !== 'function') throw new Error('expected updater')
    expect(update(defaultLocalSessionOnboardingState)).toMatchObject({
      providerChoiceComplete: true,
      backupComplete: true,
    })
  })
})
