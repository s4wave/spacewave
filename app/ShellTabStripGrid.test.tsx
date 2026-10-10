import { useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { Model, type IJsonModel } from '@aptre/flex-layout'
import type * as WebState from '@s4wave/web/state/index.js'

import { resetBrowserShellTabsStoreForTests } from './BrowserShellTabsStore.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'
import { ShellTabStrip } from './ShellFlexLayout.js'
import { useShellTabs } from './ShellTabContext.js'
import {
  installShellTabTestBrowser,
  readShellTabsSnapshot,
  seedShellTabs,
  type ShellTabTestBrowser,
} from './ShellTabTestHarness.js'
import {
  SHELL_GRID_BASE_MODEL,
  applyLocalStateToModel,
  encodeGridLayout,
} from './shell-grid-utils.js'

// Keep the real FlexLayout model so a grid has no tabset named shell-tabset,
// and replace only the DOM layout that renders it.
const mockLayoutModels = vi.hoisted(() => [] as Model[])

vi.mock('@aptre/flex-layout', async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  OptimizedLayout: function OptimizedLayout({
    model,
    onModelChange,
  }: {
    model: Model
    onModelChange?: (model: Model) => void
  }) {
    // Report every action to the shell as the DOM layout does.
    useEffect(() => {
      mockLayoutModels.push(model)
      const listener = () => onModelChange?.(model)
      model.addChangeListener(listener)
      return () => model.removeChangeListener(listener)
    }, [model, onModelChange])
    return null
  },
}))

vi.mock('@s4wave/web/state/index.js', async () => {
  const [React, actual] = await Promise.all([
    import('react'),
    vi.importActual<typeof WebState>('@s4wave/web/state/index.js'),
  ])
  return {
    ...actual,
    useStateAtom: <T,>(_: unknown, __: string, initialValue: T) =>
      React.useState(initialValue),
  }
})

vi.mock('./ShellTabContent.js', () => ({ ShellTabContent: () => null }))

const continuationEntry: ShellDocumentEntry = {
  kind: 'continuation',
  path: '/',
  params: {},
  incarnation: 'test-document',
}

const gridJson: IJsonModel = {
  ...SHELL_GRID_BASE_MODEL,
  layout: {
    type: 'row',
    weight: 100,
    children: [
      {
        type: 'tabset',
        id: 'grid-left',
        weight: 50,
        selected: 0,
        children: [
          {
            type: 'tab',
            id: 'grid-home',
            name: 'Home',
            component: 'shell-panel',
          },
        ],
      },
      {
        type: 'tabset',
        id: 'grid-right',
        weight: 50,
        selected: 0,
        children: [
          {
            type: 'tab',
            id: 'grid-blog',
            name: 'Blog',
            component: 'shell-panel',
          },
        ],
      },
    ],
  },
}

function DocsCommandProbe() {
  const { activeTabId, openPathInActiveTabset } = useShellTabs()
  return (
    <button
      onClick={() => {
        openPathInActiveTabset('/docs', {
          afterTabId: activeTabId || undefined,
          focusExisting: true,
        })
      }}
      type="button"
    >
      Open Documentation
    </button>
  )
}

describe('ShellTabStrip grid layout', () => {
  let restoreTestBrowser: ShellTabTestBrowser | undefined

  beforeEach(() => {
    restoreTestBrowser = installShellTabTestBrowser()
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        disconnect() {}
      },
    )
  })
  afterEach(() => {
    cleanup()
    localStorage.clear()
    sessionStorage.clear()
    window.location.hash = ''
    restoreTestBrowser?.()
    restoreTestBrowser = undefined
    vi.unstubAllGlobals()
    mockLayoutModels.length = 0
    resetBrowserShellTabsStoreForTests()
  })

  it('selects a Documentation tab opened from the Help menu in the active pane', async () => {
    seedShellTabs([
      { id: 'grid-home', name: 'Home', path: '/' },
      { id: 'grid-blog', name: 'Blog', path: '/blog' },
    ])
    const seeded = Model.fromJson(gridJson)
    applyLocalStateToModel(seeded, {
      activeTabSetId: 'grid-left',
      tabSetSelections: { 'grid-left': 'grid-home', 'grid-right': 'grid-blog' },
    })
    window.location.hash = `#/g/${encodeGridLayout(seeded)}`

    render(
      <ShellTabStrip entry={continuationEntry}>
        <DocsCommandProbe />
      </ShellTabStrip>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Open Documentation' }))

    await waitFor(() => {
      const docs = readShellTabsSnapshot().records.find(
        (record) => record.path === '/docs',
      )
      const model = mockLayoutModels.at(-1)
      const node = model?.getNodeById(docs?.id ?? '')
      expect(node?.getParent()?.getId()).toBe('grid-left')
      expect(model?.getActiveTabset()?.getSelectedNode()).toBe(node)
    })
  })
})
