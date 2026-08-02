/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useState, useEffect, useMemo, type ReactNode } from 'react'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { getAppPath } from '@s4wave/web/router/app-path.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { KeyDispatcher } from '@s4wave/web/command/KeyDispatcher.js'
import { CommandPalette } from '@s4wave/web/command/CommandPalette.js'
import { WhichKeyPanel } from '@s4wave/web/command/WhichKeyPanel.js'
import { ShellTabFocusContextProvider } from '@s4wave/web/command/FocusContext.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import {
  DocumentTitleProvider,
  getRouteDocumentTitleParts,
  useDocumentTitle,
} from '@s4wave/web/title/DocumentTitleContext.js'
import { BuiltinCommands } from '@s4wave/app/BuiltinCommands.js'
import { CliTerminalSessionProvider } from '@s4wave/app/terminal/CliTerminalSessionProvider.js'
import { DebugCommands } from '@s4wave/app/DebugCommands.js'
import { getTabDisplayName } from './shell-tab.js'
import { ShellTabStrip } from './ShellFlexLayout.js'
import { ShellMenuBar } from './ShellMenuBar.js'
import { useShellTabs } from './ShellTabContext.js'
import {
  classifyShellDocumentEntry,
  type ShellDocumentEntry,
} from './ShellDocumentEntry.js'
import { ShellProvider } from './ShellContext.js'

// isDebug is true in debug builds (BLDR_DEBUG injected by esbuild).
const isDebug = typeof BLDR_DEBUG === 'boolean' && BLDR_DEBUG

function CommandSessionScope({ children }: { children: ReactNode }) {
  const rootResource = useRootResource()
  const { tabs, activeTabId } = useShellTabs()
  const activeSessionIndex = useMemo(() => {
    const activePath = tabs.find((tab) => tab.id === activeTabId)?.path ?? ''
    const match = activePath.match(/^\/u\/(\d+)(?:\/|$)/)
    if (!match) return null
    const sessionIndex = Number(match[1])
    return sessionIndex > 0 ? sessionIndex : null
  }, [tabs, activeTabId])
  const sessionResource = useResource(
    rootResource,
    async (root, signal, cleanup) => {
      if (activeSessionIndex === null) return null
      const result = await root.mountSessionByIdx(
        { sessionIdx: activeSessionIndex },
        signal,
      )
      return result ? cleanup(result.session) : null
    },
    [activeSessionIndex],
  )

  if (activeSessionIndex === null) return children
  return (
    <SessionContext.Provider resource={sessionResource}>
      {children}
    </SessionContext.Provider>
  )
}

function CommandRuntime({ children }: { children?: ReactNode }) {
  return (
    <ShellTabFocusContextProvider>
      <CommandSessionScope>
        <KeyDispatcher>
          <BuiltinCommands />
          {isDebug && <DebugCommands />}
          <CommandPalette />
          <WhichKeyPanel />
          {children}
        </KeyDispatcher>
      </CommandSessionScope>
    </ShellTabFocusContextProvider>
  )
}

function ShellDocumentTitle() {
  const { tabs, activeTabId } = useShellTabs()
  const activeTab = tabs.find((tab) => tab.id === activeTabId)
  useDocumentTitle(
    getRouteDocumentTitleParts(
      activeTab?.path ?? '/',
      activeTab ? getTabDisplayName(activeTab) : '',
    ),
  )
  return null
}

// EditorShell is the main application shell with FlexLayout draggable tabs.
// The FlexLayout spans the entire content area, enabling drag-to-split anywhere.
// When splits are created, it transitions to grid mode via URL.
export function EditorShell() {
  return (
    <DocumentTitleProvider>
      <CliTerminalSessionProvider>
        <EditorShellContent />
      </CliTerminalSessionProvider>
    </DocumentTitleProvider>
  )
}
function useShellDocumentEntry(): ShellDocumentEntry {
  const [entry] = useState(() => classifyShellDocumentEntry())
  return entry
}

function EditorShellContent() {
  const documentEntry = useShellDocumentEntry()
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

  // Keep ShellTabStrip (and the single OptimizedLayout it renders) mounted for
  // both URL modes. Its model handoff preserves FlexLayout's tab nodes and
  // therefore the mounted per-tab application trees.
  return (
    <ShellProvider isGridMode={isGridMode}>
      <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
        <ShellTabStrip entry={documentEntry}>
          <ShellDocumentTitle />
          <CommandRuntime>
            <ShellMenuBar />
          </CommandRuntime>
        </ShellTabStrip>
      </BottomBarRoot>
    </ShellProvider>
  )
}
