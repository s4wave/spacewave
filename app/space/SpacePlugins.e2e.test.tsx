/**
 * Browser E2E screenshots for the SpacePlugins management panel.
 *
 * Renders the panel in the same InfoCard the space details panel wraps it in
 * and captures each interactive state (installed list, empty, add form, remove
 * confirm) for visual review. Runs with no backend: the Space resource and the
 * contents watch stream are mocked.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import {
  SpacePluginLifecycleState,
  type SpaceContentsState,
} from '@s4wave/sdk/space/space.pb.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

const mocks = vi.hoisted(() => ({
  useResourceValue: vi.fn(),
  useWatchStateRpc: vi.fn(),
  addSpacePlugin: vi.fn().mockResolvedValue(undefined),
  removeSpacePlugin: vi.fn().mockResolvedValue(undefined),
}))

let contentsState: SpaceContentsState | null = null

const spaceResource = { kind: 'space' }
const contentsResource = { kind: 'contents' }
const spaceMock = {
  addSpacePlugin: mocks.addSpacePlugin,
  removeSpacePlugin: mocks.removeSpacePlugin,
}

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: mocks.useWatchStateRpc,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mocks.useResourceValue,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContext: { useContext: () => spaceResource },
  SpaceContentsContext: { useContext: () => contentsResource },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: vi.fn() },
}))

import { SpacePlugins } from './SpacePlugins.js'

// PanelFrame mimics the space details panel column that wraps SpacePlugins in
// an InfoCard on the app background, so screenshots match production chrome.
function PanelFrame() {
  return (
    <div className="bg-background" style={{ width: '380px', padding: '16px' }}>
      <InfoCard>
        <SpacePlugins />
      </InfoCard>
    </div>
  )
}

describe('SpacePlugins panel screenshots', () => {
  beforeEach(() => {
    contentsState = null
    mocks.useResourceValue.mockImplementation((res: unknown) =>
      res === spaceResource ? spaceMock : {},
    )
    mocks.useWatchStateRpc.mockImplementation(() => contentsState)
    cleanup()
  })

  it('captures the installed plugin list', async () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'spacewave-notes',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
        {
          pluginId: 'spacewave-v86',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING,
        },
        {
          pluginId: 'broken-plugin',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_FAILED,
          detail: 'fetch plugin manifest: copy failed',
        },
      ],
    }

    render(<PanelFrame />)
    await expect.element(page.getByText('spacewave-notes')).toBeInTheDocument()
    await page.screenshot({ path: 'spaceplugins-01-installed.png' })
  })

  it('captures the empty state', async () => {
    render(<PanelFrame />)
    await expect
      .element(page.getByText('No plugins installed'))
      .toBeInTheDocument()
    await page.screenshot({ path: 'spaceplugins-02-empty.png' })
  })

  it('captures the add form with suggestions', async () => {
    render(<PanelFrame />)
    await userEvent.click(page.getByLabelText('Add plugin'))
    await expect
      .element(page.getByText('Available plugins'))
      .toBeInTheDocument()
    await page.screenshot({ path: 'spaceplugins-03-add-form.png' })
  })

  it('captures the remove confirm affordance', async () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'spacewave-notes',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
      ],
    }

    render(<PanelFrame />)
    await userEvent.click(page.getByLabelText('Remove spacewave-notes'))
    await expect
      .element(page.getByText('Remove spacewave-notes?'))
      .toBeInTheDocument()
    await page.screenshot({ path: 'spaceplugins-04-remove-confirm.png' })
  })
})
