// Exercise the plugin panel with a controlled Space contents stream.
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

vi.mock('@aptre/bldr-react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@aptre/bldr-react')>()),
  useWatchStateRpc: mocks.useWatchStateRpc,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@aptre/bldr-sdk/hooks/useResource.js')
  >()),
  useResourceValue: mocks.useResourceValue,
}))

vi.mock('@s4wave/web/contexts/contexts.js', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@s4wave/web/contexts/contexts.js')
  >()),
  SpaceContext: { useContext: () => spaceResource },
  SpaceContentsContext: { useContext: () => contentsResource },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: vi.fn() },
}))

import { SpacePlugins } from './SpacePlugins.js'

// PanelFrame uses the Space details panel layout.
function PanelFrame() {
  return (
    <div className="bg-background w-95 p-4">
      <InfoCard>
        <SpacePlugins />
      </InfoCard>
    </div>
  )
}

describe('SpacePlugins panel', () => {
  beforeEach(() => {
    contentsState = null
    mocks.useResourceValue.mockImplementation((res: unknown) =>
      res === spaceResource ? spaceMock : {},
    )
    mocks.useWatchStateRpc.mockImplementation(() => contentsState)
    void cleanup()
  })

  it('shows the installed plugin list', async () => {
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

    await render(<PanelFrame />)
    await expect.element(page.getByText('spacewave-notes')).toBeInTheDocument()
  })

  it('shows the empty state', async () => {
    await render(<PanelFrame />)
    await expect
      .element(page.getByText('No plugins installed'))
      .toBeInTheDocument()
  })

  it('shows the add form with suggestions', async () => {
    await render(<PanelFrame />)
    await userEvent.click(page.getByLabelText('Add plugin'))
    await expect
      .element(page.getByText('Available plugins'))
      .toBeInTheDocument()
  })

  it('shows the remove confirm affordance', async () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'spacewave-notes',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
      ],
    }

    await render(<PanelFrame />)
    await userEvent.click(page.getByLabelText('Remove spacewave-notes'))
    await expect
      .element(page.getByText('Remove spacewave-notes?'))
      .toBeInTheDocument()
  })
})
