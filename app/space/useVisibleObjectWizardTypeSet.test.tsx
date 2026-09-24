import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

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

vi.mock('./useObjectWizards.js', () => ({
  useObjectWizards: () => ({
    value: { wizards: h.wizards },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('useVisibleObjectWizardTypeSet', () => {
  it('reacts to the runtime experimental creator preference', () => {
    vi.stubEnv('DEV', false)
    const { result } = renderHook(() => useVisibleObjectWizardTypeSet())

    expect(result.current.has('git/repo')).toBe(true)
    expect(result.current.has('forge/task')).toBe(false)

    act(() => setExperimentalCreatorsEnabled(true))

    expect(result.current.has('forge/task')).toBe(true)
  })
})
