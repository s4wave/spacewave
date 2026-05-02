import { afterEach, describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'

import { StaticProvider } from '@s4wave/app/prerender/StaticContext.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

import { QuickstartLoading } from './QuickstartLoading.js'

function renderQuickstartLoading() {
  return render(
    <RouterProvider path="/quickstart/drive" onNavigate={() => {}}>
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

  it('renders the current browser boot status', () => {
    globalThis.__swBootStatus = {
      phase: 'wasm',
      detail: 'Preparing runtime...',
      state: 'loading',
    }

    renderQuickstartLoading()

    expect(screen.getByText('Preparing runtime...')).toBeTruthy()
    expect(screen.getByText('Create a Drive')).toBeTruthy()
  })

  it('renders the default boot status before boot progress arrives', () => {
    renderQuickstartLoading()

    expect(screen.getByText('Loading application...')).toBeTruthy()
  })
})
