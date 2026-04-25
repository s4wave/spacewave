import { useState, useEffect, useMemo, type ReactNode } from 'react'

import { getAppPath } from '@s4wave/web/router/app-path.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { HashRouter } from '@s4wave/web/router/HashRouter.js'
import { Routes, Route } from '@s4wave/web/router/router.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'
import { KeyboardManager } from '@s4wave/web/command/KeyboardManager.js'
import { CommandPalette } from '@s4wave/web/command/CommandPalette.js'
import { BuiltinCommands } from '@s4wave/app/BuiltinCommands.js'
import { DebugCommands } from '@s4wave/app/DebugCommands.js'
import {
  TabContextProvider,
  type TabContextValue,
} from '@s4wave/web/object/TabContext.js'

import { ShellTabStrip } from './ShellFlexLayout.js'
import { ShellGridLayout } from './ShellGridLayout.js'
import { ShellMenuBar } from './ShellMenuBar.js'
import {
  ShellTabsProvider,
  useShellTabs,
  ShellTabStateProvider,
} from './ShellTabContext.js'
import { ShellProvider } from './ShellContext.js'

// isDebug is true in debug builds (BLDR_DEBUG injected by esbuild).
const isDebug = typeof BLDR_DEBUG === 'boolean' && BLDR_DEBUG

// noop stubs for TabContextValue in the command scope.
const noopAddTab = () => Promise.resolve({ tabId: '' })
const noopNavigateTab = () => Promise.resolve({})

// ActiveTabCommandScope provides command system components scoped to
// the currently active tab. Wraps children in ShellTabStateProvider
// and TabContextProvider so useTabId() works from either context.
function ActiveTabCommandScope({ children }: { children: ReactNode }) {
  const { activeTabId } = useShellTabs()
  const tabContext = useMemo<TabContextValue>(
    () => ({
      tabId: activeTabId,
      addTab: noopAddTab,
      navigateTab: noopNavigateTab,
    }),
    [activeTabId],
  )
  return (
    <ShellTabStateProvider tabId={activeTabId}>
      <TabContextProvider value={tabContext}>
        <KeyboardManager />
        <BuiltinCommands />
        {isDebug && <DebugCommands />}
        <CommandPalette />
        {children}
      </TabContextProvider>
    </ShellTabStateProvider>
  )
}

// EditorShell is the main application shell with FlexLayout draggable tabs.
// The FlexLayout spans the entire content area, enabling drag-to-split anywhere.
// When splits are created, it transitions to grid mode via URL.
export function EditorShell() {
  const namespace = useStateNamespace(['shell'])

  const [openMenu, setOpenMenu] = useStateAtom<string>(
    namespace,
    'openMenu',
    '',
  )

  // Track grid mode state with proper reactivity to hash changes
  const [isGridMode, setIsGridMode] = useState(() => {
    return getAppPath().startsWith('/g/')
  })

  // Listen for hash changes to update grid mode state
  useEffect(() => {
    const handleHashChange = () => {
      setIsGridMode(getAppPath().startsWith('/g/'))
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  // In grid mode, render ShellGridLayout directly without ShellTabStrip's Layout.
  // ShellTabStateProvider scopes command components to the active tab so
  // useTabId() works for KeyboardManager, BuiltinCommands, and CommandPalette.
  if (isGridMode) {
    return (
      <ShellProvider isGridMode={true}>
        <ShellTabsProvider>
          <ActiveTabCommandScope>
            <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
              <div className="flex h-full flex-1 flex-col overflow-hidden">
                {/* Header: menu bar */}
                <div className="bg-topbar-back h-shell-header relative flex shrink-0 items-center">
                  <ShellMenuBar />
                </div>
                {/* Grid layout */}
                <HashRouter>
                  <Routes>
                    <Route path="/g/:layoutData">
                      <ShellGridLayout />
                    </Route>
                    <Route path="*">
                      <NavigatePath to="/" replace />
                    </Route>
                  </Routes>
                </HashRouter>
              </div>
            </BottomBarRoot>
          </ActiveTabCommandScope>
        </ShellTabsProvider>
      </ShellProvider>
    )
  }

  // Normal mode: ShellTabStrip handles routing via FlexLayout.
  // ShellTabStrip provides ShellTabsProvider and ShellTabStateProvider internally,
  // so command components placed as children have access to useTabId().
  return (
    <ShellProvider isGridMode={false}>
      <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
        <ShellTabStrip>
          <ShellMenuBar />
          <KeyboardManager />
          <BuiltinCommands />
          {isDebug && <DebugCommands />}
          <CommandPalette />
        </ShellTabStrip>
      </BottomBarRoot>
    </ShellProvider>
  )
}
