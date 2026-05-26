import { afterEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'

import { StaticProvider } from '@s4wave/app/prerender/StaticContext.js'
import { browserStartupPhaseRail } from '@s4wave/app/loading/status/browser-startup-model.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

import { QuickstartLoading } from './QuickstartLoading.js'

function renderQuickstartLoading(path = '/quickstart/drive') {
  return render(
    <RouterProvider path={path} onNavigate={() => {}}>
      <StaticProvider>
        <QuickstartLoading />
      </StaticProvider>
    </RouterProvider>,
  )
}

describe('QuickstartLoading', () => {
  afterEach(() => {
    globalThis.__swBootStatus = undefined
  })

  it('renders the projected browser startup phase', () => {
    globalThis.__swBootStatus = {
      phase: 'wasm',
      detail: 'Preparing runtime...',
      state: 'loading',
    }

    renderQuickstartLoading()

    expect(screen.getByText('Connect: Connecting the app shell.')).toBeTruthy()
    expect(screen.getByText('30%')).toBeTruthy()
    expect(screen.getByText('Create a Drive')).toBeTruthy()
    expect(screen.getByLabelText('Startup phases')).toBeTruthy()
    for (const phase of browserStartupPhaseRail) {
      expect(screen.getByText(phase.label)).toBeTruthy()
    }
  })

  it('renders the default startup projection before boot progress arrives', () => {
    renderQuickstartLoading()

    expect(screen.getByText('Prepare: Preparing browser files.')).toBeTruthy()
  })

  it('treats dynamic quickstart ids as unknown public routes', () => {
    renderQuickstartLoading('/quickstart/glados-workspace')

    expect(screen.getByText('Unknown quickstart option.')).toBeTruthy()
  })
})
