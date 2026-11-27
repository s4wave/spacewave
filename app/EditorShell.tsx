/* eslint-disable react-doctor/rerender-state-only-in-handlers */
import { useState, useEffect, useMemo, type ReactNode } from 'react'

import {
  useAppEnvironment,
  useAppNavigation,
  type AppEnvironment,
} from '@s4wave/web/sdk/app/environment.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { useStateAtom, useStateNamespace } from '@s4wave/web/state/index.js'
import { KeyDispatcher } from '@s4wave/web/command/KeyDispatcher.js'
import { CommandPalette } from '@s4wave/web/command/CommandPalette.js'
import { WhichKeyPanel } from '@s4wave/web/command/WhichKeyPanel.js'
import { ShellTabFocusContextProvider } from '@s4wave/web/command/FocusContext.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useSessionMount } from '@s4wave/app/hooks/useSharedMounts.js'
import {
  DocumentTitleProvider,
  getRouteDocumentTitleParts,
  useDocumentTitle,
} from '@s4wave/web/title/DocumentTitleContext.js'
import { BuiltinCommands } from '@s4wave/app/BuiltinCommands.js'
import { DebugCommands } from '@s4wave/app/DebugCommands.js'
import { getTabDisplayName } from './shell-tab.js'
import { ShellTabStrip } from './ShellFlexLayout.js'
import { ShellMenuBar } from './ShellMenuBar.js'
import { useShellTabs } from './ShellTabContext.js'
import {
  classifyShellDocumentEntry,
  resolveShellDocumentEntry,
  type ShellDocumentEntry,
} from './ShellDocumentEntry.js'
import { ShellProvider } from './ShellContext.js'

// isDebug is true in debug builds (BLDR_DEBUG injected by esbuild).
const isDebug = typeof BLDR_DEBUG === 'boolean' && BLDR_DEBUG

/** CommandSessionScope updates the session resource without remounting command UI. */
function CommandSessionScope({ children }: { children: ReactNode }) {
  // Read the committed active tab path for the command session.
  const rootResource = useRootResource()
  const { tabs, activeTabId } = useShellTabs()
  const activeSessionIndex = useMemo(() => {
    // Identify the session referenced by the committed active tab.
    const activePath = tabs.find((tab) => tab.id === activeTabId)?.path ?? ''
    const match = activePath.match(/^\/u\/(\d+)(?:\/|$)/)
    if (!match) return null
    const sessionIndex = Number(match[1])
    return sessionIndex > 0 ? sessionIndex : null
  }, [tabs, activeTabId])

  // Share the active tab's Session mount. The tab remounts it when the
  // Session moves to another account.
  const sessionResource = useSessionMount(rootResource, activeSessionIndex)

  // Keep command UI mounted when a tab enters or leaves a session.
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
// When splits are created, the shell transitions to grid mode via URL.
export function EditorShell() {
  const environment = useAppEnvironment()
  return (
    <DocumentTitleProvider enabled={!environment.id}>
      <EditorShellContent />
    </DocumentTitleProvider>
  )
}

// getDocumentStorage returns the environment's document storage, or undefined
// where the browser denies it.
function getDocumentStorage(environment: AppEnvironment): Storage | undefined {
  try {
    return environment.documentStorage
  } catch {
    return undefined
  }
}

function useShellDocumentEntry(): ShellDocumentEntry {
  const environment = useAppEnvironment()
  const [entry] = useState(() =>
    resolveShellDocumentEntry(getDocumentStorage(environment), () =>
      classifyShellDocumentEntry(
        environment.id
          ? {
              ...environment.navigation.getAppNavigation(),
              navigationType: 'navigate',
              incarnation: environment.id,
            }
          : undefined,
      ),
    ),
  )
  return entry
}

function EditorShellContent() {
  // Read the document entry and persisted bottom-bar menu.
  const { getAppPath, subscribe } = useAppNavigation()
  const documentEntry = useShellDocumentEntry()
  const namespace = useStateNamespace(['shell'])
  const [openMenu, setOpenMenu] = useStateAtom<string>(
    namespace,
    'openMenu',
    '',
  )

  // Track whether navigation uses the split layout.
  const [isGridMode, setIsGridMode] = useState(() => {
    return getAppPath().startsWith('/g/')
  })

  // Keep the layout mode aligned with navigation.
  useEffect(() => {
    const handleHashChange = () => {
      setIsGridMode(getAppPath().startsWith('/g/'))
    }
    return subscribe(handleHashChange)
  }, [getAppPath, subscribe])

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
