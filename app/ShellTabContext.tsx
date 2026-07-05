import {
  createContext,
  use,
  useMemo,
  ReactNode,
  useReducer,
  useState,
  useCallback,
  useEffect,
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

// SHELL_TABS_STORAGE_KEY is the sessionStorage key for shell tabs state.
export const SHELL_TABS_STORAGE_KEY = 'shell-tabs-state'

// ShellTabContextValue provides tab information to descendant components.
export interface ShellTabContextValue {
  tabId: string
}

// ShellTabContext provides the active tab info to descendant components.
const ShellTabContext = createContext<ShellTabContextValue | null>(null)

// useShellTab returns the current tab context.
export function useShellTab(): ShellTabContextValue | null {
  return use(ShellTabContext)
}

// useTabId returns the current tab ID from context.
export function useTabId(): string | null {
  return use(ShellTabContext)?.tabId ?? null
}

// useIsTabActive returns whether the current tab is the active tab.
// Returns true if there's no tab context (not in a tab), if this is
// the active tab, or if in static prerender mode. Falls back to
// TabContext's tabId when ShellTabContext is not available.
export function useIsTabActive(): boolean {
  const isStatic = useIsStaticMode()
  const shellTabId = use(ShellTabContext)?.tabId
  const tabContextTabId = useTabContextTabId()
  const tabId = shellTabId ?? tabContextTabId
  const tabsContext = use(ShellTabsContext)
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

export interface OpenShellTabOptions {
  afterTabId?: string
  select?: boolean
  focusExisting?: boolean
}

export type ActiveTabsetPathOpener = (
  path: string,
  options?: OpenShellTabOptions,
) => string | null

export interface AddShellTabOptions {
  afterTabId?: string
  select?: boolean
}

type ShellTabsProviderAction =
  | { type: 'set_tabs'; update: React.SetStateAction<ShellTab[]> }
  | { type: 'set_active_tab_id'; update: React.SetStateAction<string> }
  | {
      type: 'open_path_in_new_tab'
      path: string
      tabId: string
      afterTabId?: string
      select: boolean
      focusExisting: boolean
    }
  | {
      type: 'add_shell_tab'
      tab: ShellTab
      afterTabId?: string
      select: boolean
    }
  | { type: 'select_tab'; tabId: string }
  | { type: 'update_tab_path'; tabId: string; path: string }
  | { type: 'update_tab_auto_name'; tabId: string; name: string }
  | { type: 'update_tab_custom_name'; tabId: string; customName?: string }
  | { type: 'retain_tabs'; tabIds: Set<string>; fallbackActiveTabId?: string }
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

function shellTabsEqual(a: ShellTab[], b: ShellTab[]): boolean {
  return (
    a.length === b.length &&
    a.every((tab, idx) => {
      const other = b[idx]
      return (
        other !== undefined &&
        tab.id === other.id &&
        tab.name === other.name &&
        tab.path === other.path &&
        tab.customName === other.customName
      )
    })
  )
}

function normalizeShellTabsState(state: ShellTabsState): ShellTabsState {
  const seen = new Set<string>()
  const tabs = state.tabs.flatMap((tab) => {
    if (
      !tab ||
      typeof tab.id !== 'string' ||
      tab.id === '' ||
      typeof tab.path !== 'string' ||
      seen.has(tab.id)
    ) {
      return []
    }
    seen.add(tab.id)
    const name =
      typeof tab.name === 'string' && tab.name !== ''
        ? tab.name
        : getTabNameFromPath(tab.path)
    const customName =
      typeof tab.customName === 'string' && tab.customName !== ''
        ? tab.customName
        : undefined
    return [{ id: tab.id, name, path: tab.path, customName }]
  })

  const normalizedTabs = tabs.length > 0 ? tabs : [DEFAULT_HOME_TAB]
  const activeTab = normalizedTabs.find((tab) => tab.id === state.activeTabId)
  return {
    tabs: normalizedTabs,
    activeTabId: activeTab?.id ?? normalizedTabs[0].id,
  }
}

function applyShellTabsState(
  state: ShellTabsProviderState,
  nextState: ShellTabsState,
): ShellTabsProviderState {
  const normalized = normalizeShellTabsState(nextState)
  const renamingTabId = normalized.tabs.some(
    (t) => t.id === state.renamingTabId,
  )
    ? state.renamingTabId
    : null
  if (
    normalized.activeTabId === state.activeTabId &&
    renamingTabId === state.renamingTabId &&
    shellTabsEqual(normalized.tabs, state.tabs)
  ) {
    return state
  }
  return {
    ...state,
    tabs: normalized.tabs,
    activeTabId: normalized.activeTabId,
    renamingTabId,
  }
}

function insertShellTab(
  tabs: ShellTab[],
  tab: ShellTab,
  afterTabId?: string,
): ShellTab[] {
  const existingIdx = tabs.findIndex((t) => t.id === tab.id)
  if (existingIdx >= 0) {
    const next = [...tabs]
    next[existingIdx] = tab
    return next
  }
  if (afterTabId) {
    const idx = tabs.findIndex((t) => t.id === afterTabId)
    if (idx >= 0) {
      const next = [...tabs]
      next.splice(idx + 1, 0, tab)
      return next
    }
  }
  return [...tabs, tab]
}

function shellTabsProviderReducer(
  state: ShellTabsProviderState,
  action: ShellTabsProviderAction,
): ShellTabsProviderState {
  switch (action.type) {
    case 'set_tabs': {
      const tabs = applyStateUpdate(state.tabs, action.update)
      return applyShellTabsState(state, {
        tabs,
        activeTabId: state.activeTabId,
      })
    }
    case 'set_active_tab_id': {
      const activeTabId = applyStateUpdate(state.activeTabId, action.update)
      if (
        activeTabId === state.activeTabId ||
        !state.tabs.some((tab) => tab.id === activeTabId)
      ) {
        return state
      }
      return { ...state, activeTabId }
    }
    case 'open_path_in_new_tab': {
      const existingTab = action.focusExisting
        ? state.tabs.find((tab) => tab.path === action.path)
        : undefined
      if (existingTab) {
        const nextTabs = state.tabs.map((tab) =>
          tab.id === existingTab.id
            ? { ...tab, name: getTabNameFromPath(action.path) }
            : tab,
        )
        return applyShellTabsState(state, {
          tabs: nextTabs,
          activeTabId: action.select ? existingTab.id : state.activeTabId,
        })
      }

      const newTab: ShellTab = {
        id: action.tabId,
        name: getTabNameFromPath(action.path),
        path: action.path,
      }
      const tabs = insertShellTab(state.tabs, newTab, action.afterTabId)
      return applyShellTabsState(state, {
        tabs,
        activeTabId: action.select ? newTab.id : state.activeTabId,
      })
    }
    case 'add_shell_tab': {
      const tabs = insertShellTab(state.tabs, action.tab, action.afterTabId)
      return applyShellTabsState(state, {
        tabs,
        activeTabId: action.select ? action.tab.id : state.activeTabId,
      })
    }
    case 'select_tab':
      if (!state.tabs.some((tab) => tab.id === action.tabId)) return state
      return { ...state, activeTabId: action.tabId }
    case 'update_tab_path': {
      const name = getTabNameFromPath(action.path)
      const tabs = state.tabs.map((tab) =>
        tab.id === action.tabId ? { ...tab, path: action.path, name } : tab,
      )
      return applyShellTabsState(state, {
        tabs,
        activeTabId: state.activeTabId,
      })
    }
    case 'update_tab_auto_name': {
      const tabs = state.tabs.map((tab) =>
        tab.id === action.tabId ? { ...tab, name: action.name } : tab,
      )
      return applyShellTabsState(state, {
        tabs,
        activeTabId: state.activeTabId,
      })
    }
    case 'update_tab_custom_name': {
      const tabs = state.tabs.map((tab) =>
        tab.id === action.tabId
          ? { ...tab, customName: action.customName }
          : tab,
      )
      return applyShellTabsState(state, {
        tabs,
        activeTabId: state.activeTabId,
      })
    }
    case 'retain_tabs': {
      const tabs = state.tabs.filter((tab) => action.tabIds.has(tab.id))
      return applyShellTabsState(state, {
        tabs,
        activeTabId: action.tabIds.has(state.activeTabId)
          ? state.activeTabId
          : (action.fallbackActiveTabId ?? state.activeTabId),
      })
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
  openPathInNewTab: (path: string, options?: OpenShellTabOptions) => string
  openPathInActiveTabset: (
    path: string,
    options?: OpenShellTabOptions,
  ) => string
  registerActiveTabsetPathOpener: (opener: ActiveTabsetPathOpener) => () => void
  addShellTab: (tab: ShellTab, options?: AddShellTabOptions) => string
  selectShellTab: (tabId: string) => void
  retainShellTabs: (tabIds: Set<string>, fallbackActiveTabId?: string) => void
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
}

// ShellTabsContext provides global tabs state to all components.
const ShellTabsContext = createContext<ShellTabsContextValue | null>(null)

// useShellTabs returns the global tabs state context.
export function useShellTabs(): ShellTabsContextValue {
  const context = use(ShellTabsContext)
  if (!context) {
    throw new Error('useShellTabs must be used within a ShellTabsProvider')
  }
  return context
}

// loadTabsFromStorage loads tabs state from sessionStorage.
function loadTabsFromStorage(): ShellTabsState {
  try {
    const stored = sessionStorage.getItem(SHELL_TABS_STORAGE_KEY)
    if (stored) {
      const parsed = JSON.parse(stored) as ShellTabsState
      if (parsed.tabs?.length > 0) {
        return normalizeShellTabsState(parsed)
      }
    }
  } catch {
    // Ignore parse errors
  }
  return { tabs: [DEFAULT_HOME_TAB], activeTabId: DEFAULT_HOME_TAB.id }
}

// saveTabsToStorage saves tabs state to sessionStorage.
function saveTabsToStorage(state: ShellTabsState): void {
  try {
    sessionStorage.setItem(SHELL_TABS_STORAGE_KEY, JSON.stringify(state))
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
  const [activeTabsetPathOpener, setActiveTabsetPathOpener] =
    useState<ActiveTabsetPathOpener | null>(null)

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

  const selectShellTab = useCallback((tabId: string) => {
    dispatch({ type: 'select_tab', tabId })
  }, [])

  const openPathInNewTab = useCallback(
    (path: string, options: OpenShellTabOptions = {}) => {
      const existingTab = options.focusExisting
        ? tabs.find((tab) => tab.path === path)
        : undefined
      const tabId = existingTab?.id ?? generateTabId()
      dispatch({
        type: 'open_path_in_new_tab',
        path,
        tabId,
        afterTabId: options.afterTabId,
        select: options.select ?? true,
        focusExisting: options.focusExisting ?? false,
      })
      return tabId
    },
    [tabs],
  )

  const registerActiveTabsetPathOpener = useCallback(
    (opener: ActiveTabsetPathOpener) => {
      setActiveTabsetPathOpener(() => opener)
      return () => {
        setActiveTabsetPathOpener((current: ActiveTabsetPathOpener | null) =>
          current === opener ? null : current,
        )
      }
    },
    [],
  )

  const openPathInActiveTabset = useCallback(
    (path: string, options: OpenShellTabOptions = {}) => {
      const tabId = activeTabsetPathOpener?.(path, options)
      if (tabId) return tabId
      return openPathInNewTab(path, options)
    },
    [activeTabsetPathOpener, openPathInNewTab],
  )

  const addShellTab = useCallback(
    (tab: ShellTab, options: AddShellTabOptions = {}) => {
      dispatch({
        type: 'add_shell_tab',
        tab,
        afterTabId: options.afterTabId,
        select: options.select ?? false,
      })
      return tab.id
    },
    [],
  )

  const retainShellTabs = useCallback(
    (tabIds: Set<string>, fallbackActiveTabId?: string) => {
      dispatch({ type: 'retain_tabs', tabIds, fallbackActiveTabId })
    },
    [],
  )

  // Persist to sessionStorage when state changes.
  useEffect(() => {
    saveTabsToStorage({ tabs, activeTabId })
  }, [tabs, activeTabId])

  // Helper to update a specific tab's path
  const updateTabPath = useCallback((tabId: string, path: string) => {
    dispatch({ type: 'update_tab_path', tabId, path })
  }, [])

  const updateTabAutoName = useCallback((tabId: string, name: string) => {
    dispatch({ type: 'update_tab_auto_name', tabId, name })
  }, [])

  // Helper to update a specific tab's custom name
  const updateTabName = useCallback((tabId: string, customName: string) => {
    const nextCustomName = customName || undefined
    dispatch({
      type: 'update_tab_custom_name',
      tabId,
      customName: nextCustomName,
    })
  }, [])

  const value = useMemo<ShellTabsContextValue>(
    () => ({
      tabs,
      setTabs,
      activeTabId,
      setActiveTabId,
      openPathInNewTab,
      openPathInActiveTabset,
      registerActiveTabsetPathOpener,
      addShellTab,
      selectShellTab,
      retainShellTabs,
      updateTabPath,
      updateTabName,
      updateTabAutoName,
      renamingTabId,
      startRenaming,
      stopRenaming,
    }),
    [
      tabs,
      setTabs,
      activeTabId,
      setActiveTabId,
      openPathInNewTab,
      openPathInActiveTabset,
      registerActiveTabsetPathOpener,
      addShellTab,
      selectShellTab,
      retainShellTabs,
      updateTabPath,
      updateTabName,
      updateTabAutoName,
      renamingTabId,
      startRenaming,
      stopRenaming,
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
