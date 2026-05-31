import React from 'react'
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { QuickstartRegistration } from '@s4wave/sdk/quickstart/registry/registry.pb.js'
import type { Root } from '@s4wave/sdk/root'

import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
} from '../creator-visibility.js'
import {
  QuickstartOptionsProvider,
  useVisibleQuickstartOptions,
} from './useQuickstartOptions.js'

const h = vi.hoisted(() => ({
  registrations: [] as QuickstartRegistration[],
}))

vi.mock('@s4wave/web/hooks/useDynamicRegistrations.js', () => ({
  useDynamicRegistrations: () => h.registrations,
}))

function QuickstartProbe() {
  const options = useVisibleQuickstartOptions()
  return (
    <div>
      {options.map((option) => (
        <span key={option.id}>{option.id}</span>
      ))}
    </div>
  )
}

function renderProvider() {
  return render(
    <QuickstartOptionsProvider
      rootResource={{
        value: {} as Root,
        loading: false,
        error: null,
        retry: vi.fn(),
      }}
    >
      <QuickstartProbe />
    </QuickstartOptionsProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
  h.registrations = []
})

describe('QuickstartOptionsProvider', () => {
  it('hides experimental static and dynamic Quickstarts in release by default', () => {
    vi.stubEnv('DEV', false)
    h.registrations = [
      {
        quickstartId: 'experimental-dynamic',
        pluginId: 'plugin',
        name: 'Experimental Dynamic',
        description: 'Experimental workspace',
        category: 'tools',
        experimental: true,
      },
    ]

    renderProvider()

    expect(screen.queryByText('device')).toBeNull()
    expect(screen.queryByText('experimental-dynamic')).toBeNull()
    expect(screen.getByText('drive')).toBeTruthy()
  })

  it('reacts to release browser opt-in for static and dynamic Quickstarts', () => {
    vi.stubEnv('DEV', false)
    h.registrations = [
      {
        quickstartId: 'experimental-dynamic',
        pluginId: 'plugin',
        name: 'Experimental Dynamic',
        description: 'Experimental workspace',
        category: 'tools',
        experimental: true,
      },
    ]

    renderProvider()

    expect(screen.queryByText('device')).toBeNull()

    act(() => setExperimentalCreatorsEnabled(true))

    expect(screen.getByText('device')).toBeTruthy()
    expect(screen.getByText('experimental-dynamic')).toBeTruthy()
  })
})
