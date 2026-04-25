import {
  createContext,
  useContext,
  useMemo,
  ReactNode,
  useReducer,
  useState,
  useCallback,
  useEffect,
  useRef,
} from 'react'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'
import { TabActiveProvider } from '@s4wave/web/contexts/TabActiveContext.js'
import {
  StateNamespaceProvider,
  atomWithLocalStorage,
  type Atom,
  type StateType,
} from '@s4wave/web/state/index.js'
import {
  ShellTab,
  DEFAULT_HOME_TAB,
  getTabNameFromPath,
  generateTabId,
} from '@s4wave/app/shell-tab.js'
import { useTabId as useTabContextTabId } from '@s4wave/web/object/TabContext.js'

// TAB_STATE_PREFIX is the localStorage key prefix for tab-specific state.
export const TAB_STATE_PREFIX = 'tab-state-'

// SHELL_TABS_STORAGE_KEY is the localStorage key for shell tabs state.
export const SHELL_TABS_STORAGE_KEY = 'shell-tabs-state'

// ShellTabContextValue provides tab information to descendant components.
export interface ShellTabContextValue {
  tabId: string
}

// ShellTabContext provides the active tab info to descendant components.
const ShellTabContext = createContext<ShellTabContextValue | null>(null)

// useShellTab returns the current tab context.
export function useShellTab(): ShellTabContextValue | null {
  return useContext(ShellTabContext)
}

// useTabId returns the current tab ID from context.
export function useTabId(): string | null {
  return useContext(ShellTabContext)?.tabId ?? null
}

// useIsTabActive returns whether the current tab is the active tab.
// Returns true if there's no tab context (not in a tab), if this is
// the active tab, or if in static prerender mode. Falls back to
// TabContext's tabId when ShellTabContext is not available.
export function useIsTabActive(): boolean {
  const isStatic = useIsStaticMode()
  const shellTabId = useContext(ShellTabContext)?.tabId
  const tabContextTabId = useTabContextTabId()
  const tabId = shellTabId ?? tabContextTabId
  const tabsContext = useContext(ShellTabsContext)
  if (isStatic) return true
  if (!tabId || !tabsContext) return true
  return tabsContext.activeTabId === tabId
}

// ShellTabsState contains the global shell tabs state.
export interface ShellTabsState {
  tabs: ShellTab[]
  activeTabId: string
}

interface ShellTabsProviderState extends ShellTabsState {
  renamingTabId: string | null
}

type ShellTabsProviderAction =
  | { type: 'hydrate'; state: ShellTabsState }
  | { type: 'set_tabs'; update: React.SetStateAction<ShellTab[]> }
  | { type: 'set_active_tab_id'; update: React.SetStateAction<string> }
  | { type: 'start_renaming'; tabId: string }
  | { type: 'stop_renaming' }

function isStateUpdater<T>(
  update: React.SetStateAction<T>,
): update is (prevState: T) => T {
  return typeof update === 'function'
}

function applyStateUpdate<T>(state: T, update: React.SetStateAction<T>): T {
  if (isStateUpdater(update)) {
    return update(state)
  }
  return update
}

function shellTabsProviderReducer(
  state: ShellTabsProviderState,
  action: ShellTabsProviderAction,
): ShellTabsProviderState {
  switch (action.type) {
    case 'hydrate': {
      const activeTabId =
        action.state.tabs.some((t) => t.id === state.activeTabId) ?
          state.activeTabId
        : action.state.tabs[0]?.id || DEFAULT_HOME_TAB.id
      const renamingTabId =
        action.state.tabs.some((t) => t.id === state.renamingTabId) ?
          state.renamingTabId
        : null
      return { ...action.state, activeTabId, renamingTabId }
    }
    case 'set_tabs': {
      const tabs = applyStateUpdate(state.tabs, action.update)
      if (tabs === state.tabs) {
        return state
      }
      const renamingTabId =
        tabs.some((t) => t.id === state.renamingTabId) ?
          state.renamingTabId
        : null
      if (renamingTabId === state.renamingTabId) {
        return { ...state, tabs }
      }
      return { ...state, tabs, renamingTabId }
    }
    case 'set_active_tab_id': {
      const activeTabId = applyStateUpdate(state.activeTabId, action.update)
      if (activeTabId === state.activeTabId) {
        return state
      }
      return {
        ...state,
        activeTabId,
      }
    }
    case 'start_renaming':
      return { ...state, renamingTabId: action.tabId }
    case 'stop_renaming':
      return { ...state, renamingTabId: null }
  }
}

function initializeShellTabsProviderState(): ShellTabsProviderState {
  const stored = loadTabsFromStorage()
  return { ...stored, renamingTabId: null }
}

// ShellTabsContextValue provides access to global tabs state.
export interface ShellTabsContextValue {
  tabs: ShellTab[]
  setTabs: React.Dispatch<React.SetStateAction<ShellTab[]>>
  activeTabId: string
  setActiveTabId: React.Dispatch<React.SetStateAction<string>>
  updateTabPath: (tabId: string, path: string) => void
  // updateTabName sets a custom name for a tab. Empty string clears it.
  updateTabName: (tabId: string, customName: string) => void
  // updateTabAutoName updates the auto-derived name for a tab without
  // overriding a user-set customName.
  updateTabAutoName: (tabId: string, name: string) => void
  // renamingTabId is the ID of the tab currently being renamed, or null.
  renamingTabId: string | null
  // startRenaming triggers inline rename for the given tab ID.
  startRenaming: (tabId: string) => void
  // stopRenaming clears the renaming state.
  stopRenaming: () => void
  // Subscribe to external tab changes (from other windows)
  subscribeToExternalChanges: (
    callback: (tabs: ShellTab[]) => void,
  ) => () => void
}

// ShellTabsContext provides global tabs state to all components.
const ShellTabsContext = createContext<ShellTabsContextValue | null>(null)

// useShellTabs returns the global tabs state context.
export function useShellTabs(): ShellTabsContextValue {
  const context = useContext(ShellTabsContext)
  if (!context) {
    throw new Error('useShellTabs must be used within a ShellTabsProvider')
  }
  return context
}

// loadTabsFromStorage loads tabs state from localStorage.
function loadTabsFromStorage(): ShellTabsState {
  try {
    const stored = localStorage.getItem(SHELL_TABS_STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as ShellTabsState
      if (parsed.tabs?.length > 0) {
        return parsed
      }
    }
  } catch {
    // Ignore parse errors
  }
  return { tabs: [DEFAULT_HOME_TAB], activeTabId: DEFAULT_HOME_TAB.id }
}

// saveTabsToStorage saves tabs state to localStorage.
function saveTabsToStorage(state: ShellTabsState): void {
  try {
    localStorage.setItem(SHELL_TABS_STORAGE_KEY, JSON.stringify(state))
  } catch {
    // Ignore storage errors
  }
}

// ShellTabsProvider provides global tabs state to all components.
export function ShellTabsProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(
    shellTabsProviderReducer,
    undefined,
    initializeShellTabsProviderState,
  )
  const { tabs, activeTabId, renamingTabId } = state

  const setTabs = useCallback((update: React.SetStateAction<ShellTab[]>) => {
    dispatch({ type: 'set_tabs', update })
  }, [])
  const setActiveTabId = useCallback((update: React.SetStateAction<string>) => {
    dispatch({ type: 'set_active_tab_id', update })
  }, [])
  const startRenaming = useCallback((tabId: string) => {
    dispatch({ type: 'start_renaming', tabId })
  }, [])
  const stopRenaming = useCallback(() => {
    dispatch({ type: 'stop_renaming' })
  }, [])

  // Subscribers for external tab changes
  const externalChangeSubscribersRef = useRef<Set<(tabs: ShellTab[]) => void>>(
    new Set(),
  )

  // Persist to localStorage when state changes
  useEffect(() => {
    saveTabsToStorage({ tabs, activeTabId })
  }, [tabs, activeTabId])

  // Listen for cross-window storage changes
  useEffect(() => {
    const handleStorage = (e: StorageEvent) => {
      if (e.key !== SHELL_TABS_STORAGE_KEY || !e.newValue) return
      try {
        const parsed = JSON.parse(e.newValue) as ShellTabsState
        if (parsed.tabs?.length > 0) {
          dispatch({ type: 'hydrate', state: parsed })
          // Notify subscribers of external change
          for (const callback of externalChangeSubscribersRef.current) {
            callback(parsed.tabs)
          }
        }
      } catch {
        // Ignore parse errors
      }
    }
    window.addEventListener('storage', handleStorage)
    return () => window.removeEventListener('storage', handleStorage)
  }, [])

  // Helper to update a specific tab's path
  const updateTabPath = useCallback(
    (tabId: string, path: string) => {
      const name = getTabNameFromPath(path)
      setTabs((prev) => {
        const idx = prev.findIndex((t) => t.id === tabId)
        const tab = idx >= 0 ? prev[idx] : null
        if (!tab) return prev
        if (tab.path === path && tab.name === name) return prev
        const next = [...prev]
        next[idx] = { ...tab, path, name }
        return next
      })
    },
    [setTabs],
  )

  const updateTabAutoName = useCallback(
    (tabId: string, name: string) => {
      setTabs((prev) => {
        const idx = prev.findIndex((t) => t.id === tabId)
        const tab = idx >= 0 ? prev[idx] : null
        if (!tab) return prev
        if (tab.name === name) return prev
        const next = [...prev]
        next[idx] = { ...tab, name }
        return next
      })
    },
    [setTabs],
  )

  // Helper to update a specific tab's custom name
  const updateTabName = useCallback(
    (tabId: string, customName: string) => {
      const nextCustomName = customName || undefined
      setTabs((prev) => {
        const idx = prev.findIndex((t) => t.id === tabId)
        const tab = idx >= 0 ? prev[idx] : null
        if (!tab) return prev
        if (tab.customName === nextCustomName) return prev
        const next = [...prev]
        next[idx] = { ...tab, customName: nextCustomName }
        return next
      })
    },
    [setTabs],
  )

  // Subscribe to external tab changes
  const subscribeToExternalChanges = useCallback(
    (callback: (tabs: ShellTab[]) => void) => {
      externalChangeSubscribersRef.current.add(callback)
      return () => {
        externalChangeSubscribersRef.current.delete(callback)
      }
    },
    [],
  )

  const value = useMemo<ShellTabsContextValue>(
    () => ({
      tabs,
      setTabs,
      activeTabId,
      setActiveTabId,
      updateTabPath,
      updateTabName,
      updateTabAutoName,
      renamingTabId,
      startRenaming,
      stopRenaming,
      subscribeToExternalChanges,
    }),
    [
      tabs,
      setTabs,
      activeTabId,
      setActiveTabId,
      updateTabPath,
      updateTabName,
      updateTabAutoName,
      renamingTabId,
      startRenaming,
      stopRenaming,
      subscribeToExternalChanges,
    ],
  )

  return (
    <ShellTabsContext.Provider value={value}>
      {children}
    </ShellTabsContext.Provider>
  )
}

// ShellTabStateProvider provides tab-specific state to descendant components.
// Each tab gets its own localStorage-backed atom for persistent state.
export function ShellTabStateProvider({
  tabId,
  children,
}: {
  tabId: string
  children: ReactNode
}) {
  // Cache atoms by tab ID using useState with lazy initialization
  // This avoids accessing refs during render
  const [atomCache] = useState(() => new Map<string, Atom<StateType>>())

  const tabStateAtom = useMemo(() => {
    const cached = atomCache.get(tabId)
    if (cached) return cached

    const atom = atomWithLocalStorage<StateType>(
      `${TAB_STATE_PREFIX}${tabId}`,
      {},
    )
    atomCache.set(tabId, atom)
    return atom
  }, [atomCache, tabId])

  const contextValue = useMemo<ShellTabContextValue>(() => ({ tabId }), [tabId])

  return (
    <ShellTabContext.Provider value={contextValue}>
      <TabActiveBridge>
        <StateNamespaceProvider rootAtom={tabStateAtom}>
          {children}
        </StateNamespaceProvider>
      </TabActiveBridge>
    </ShellTabContext.Provider>
  )
}

// TabActiveBridge reads the shell tab context and provides tab-active state
// to web/ components via TabActiveProvider.
function TabActiveBridge({ children }: { children: ReactNode }) {
  const isActive = useIsTabActive()
  return <TabActiveProvider value={isActive}>{children}</TabActiveProvider>
}

// cleanupOrphanedTabStorage removes localStorage entries for tabs that no longer exist.
export function cleanupOrphanedTabStorage(activeTabIds: string[]): void {
  const activeSet = new Set(activeTabIds)
  const keysToRemove: string[] = []

  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (key?.startsWith(TAB_STATE_PREFIX)) {
      const tabId = key.slice(TAB_STATE_PREFIX.length)
      if (!activeSet.has(tabId)) {
        keysToRemove.push(key)
      }
    }
  }

  for (const key of keysToRemove) {
    localStorage.removeItem(key)
  }
}

// getTabById returns the tab with the given ID, or undefined if not found.
export function getTabById(
  tabs: ShellTab[],
  tabId: string,
): ShellTab | undefined {
  return tabs.find((t) => t.id === tabId)
}

// addTab creates a new tab and adds it to the tabs list.
export function addTab(
  tabs: ShellTab[],
  path: string,
  afterTabId?: string,
): { tabs: ShellTab[]; newTab: ShellTab } {
  const newTab: ShellTab = {
    id: generateTabId(),
    name: getTabNameFromPath(path),
    path,
  }

  if (afterTabId) {
    const index = tabs.findIndex((t) => t.id === afterTabId)
    if (index >= 0) {
      const newTabs = [...tabs]
      newTabs.splice(index + 1, 0, newTab)
      return { tabs: newTabs, newTab }
    }
  }

  return { tabs: [...tabs, newTab], newTab }
}
