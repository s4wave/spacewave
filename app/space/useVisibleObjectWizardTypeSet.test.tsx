import React from 'react'
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SpaceContext } from '@s4wave/web/contexts/contexts.js'
import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import { useVisibleObjectWizardTypeSet } from './useVisibleObjectWizardTypeSet.js'

const h = vi.hoisted(() => ({
  wizards: [
    {
      typeId: 'git/repo',
      displayName: 'Git Repository',
      persistent: true,
      wizardTypeId: 'wizard/git/repo',
    },
    {
      typeId: 'forge/task',
      displayName: 'Forge Task',
      persistent: true,
      wizardTypeId: 'wizard/forge/task',
      experimental: true,
    },
  ],
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: { wizards: h.wizards },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <SpaceContext.Provider
      resource={{
        value: {} as never,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    >
      {children}
    </SpaceContext.Provider>
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('useVisibleObjectWizardTypeSet', () => {
  it('reacts to the runtime experimental creator preference', () => {
    vi.stubEnv('DEV', false)
    const { result } = renderHook(() => useVisibleObjectWizardTypeSet(), {
      wrapper,
    })

    expect(result.current.has('git/repo')).toBe(true)
    expect(result.current.has('forge/task')).toBe(false)

    act(() => setExperimentalCreatorsEnabled(true))

    expect(result.current.has('forge/task')).toBe(true)
  })
})
