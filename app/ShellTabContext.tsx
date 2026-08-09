import {
  createContext,
  use,
  useMemo,
  useState,
  useCallback,
  useEffect,
  useSyncExternalStore,
  useRef,
} from 'react'

import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'
import { getAppNavigation, setAppPath } from '@s4wave/web/router/app-path.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useTabId as useTabContextTabId } from '@s4wave/web/object/TabContext.js'
import {
  ShellTab,
  DEFAULT_HOME_TAB,
  getTabNameFromPath,
  generateTabId,
} from '@s4wave/app/shell-tab.js'

import {
  BrowserShellTabsStore,
  BrowserShellTabsStoreError,
  getBrowserShellTabsStore,
  type BrowserShellTabRecord,
} from './BrowserShellTabsStore.js'
import {
  classifyShellDocumentEntry,
  type ShellDocumentEntry,
} from './ShellDocumentEntry.js'
import {
  clearObsoleteShellTabsState,
  readShellDocumentState,
  removeObsoleteShellTabState,
  removeShellTabDocumentState,
  writeShellDocumentState,
} from './ShellDocumentState.js'

export interface ShellTabContextValue {
  tabId: string
}

export const ShellTabContext = createContext<ShellTabContextValue | null>(null)

export function useShellTab(): ShellTabContextValue | null {
  return use(ShellTabContext)
}

export function useTabId(): string | null {
  return use(ShellTabContext)?.tabId ?? null
}

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

export interface ShellTabsState {
  tabs: ShellTab[]
  activeTabId: string
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
  onCommitted?: () => void
}

function insertTabId(
  order: string[],
  id: string,
  afterTabId?: string,
): string[] {
  const withoutId = order.filter((candidate) => candidate !== id)
  if (afterTabId) {
    const index = withoutId.indexOf(afterTabId)
    if (index >= 0) {
      withoutId.splice(index + 1, 0, id)
      return withoutId
    }
  }
  withoutId.push(id)
  return withoutId
}

function normalizeLocalOrder(
  order: string[],
  records: BrowserShellTabRecord[],
): string[] {
  const ids = new Set(records.map((record) => record.id))
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const id of order) {
    if (ids.has(id) && !seen.has(id)) {
      normalized.push(id)
      seen.add(id)
    }
  }
  for (const record of records.toSorted(
    (a, b) => a.creationSequence - b.creationSequence,
  )) {
    if (!seen.has(record.id)) {
      normalized.push(record.id)
      seen.add(record.id)
    }
  }
  return normalized
}
// SHELL_TAB_PATH_COMMITTED_EVENT fires after shared tab storage commits a path.
export const SHELL_TAB_PATH_COMMITTED_EVENT = 's4wave:shell-tab-path-committed'

export interface ShellTabsContextValue {
  tabs: ShellTab[]
  setTabs: React.Dispatch<React.SetStateAction<ShellTab[]>>
  activeTabId: string
  setActiveTabId: React.Dispatch<React.SetStateAction<string>>
  documentIncarnation: string
  openPathInNewTab: (path: string, options?: OpenShellTabOptions) => string
  openPathInActiveTabset: (
    path: string,
    options?: OpenShellTabOptions,
  ) => string
  registerActiveTabsetPathOpener: (opener: ActiveTabsetPathOpener) => () => void
  addShellTab: (tab: ShellTab, options?: AddShellTabOptions) => string
  selectShellTab: (tabId: string) => void
  retainShellTabs: (tabIds: Set<string>, fallbackActiveTabId?: string) => void
  closeShellTab: (tabId: string) => void
  resetShellTabs: () => void
  updateTabPath: (
    tabId: string,
    path: string,
    onCommitted?: () => void,
  ) => Promise<boolean>
  updateTabName: (tabId: string, customName: string) => void
  updateTabAutoName: (tabId: string, name: string) => void
  renamingTabId: string | null
  startRenaming: (tabId: string) => void
  stopRenaming: () => void
  mutationError: BrowserShellTabsStoreError | null
}

export const ShellTabsContext = createContext<ShellTabsContextValue | null>(
  null,
)

export function useShellTabs(): ShellTabsContextValue {
  const context = use(ShellTabsContext)
  if (!context) {
    throw new Error('useShellTabs must be used within a ShellTabsProvider')
  }
  return context
}

function recordAsShellTab(record: BrowserShellTabRecord): ShellTab {
  return {
    id: record.id,
    name: record.name,
    path: record.path,
    customName: record.customName,
  }
}

function useProviderStore(store: BrowserShellTabsStore) {
  return useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    store.getSnapshot,
  )
}

function removeShellTabLocalState(
  incarnation: string,
  tabId: string,
  removeObsoleteState = false,
): void {
  removeShellTabDocumentState(incarnation, tabId)
  if (removeObsoleteState) removeObsoleteShellTabState(tabId)
}

// useShellTabsContextValue owns this document's active ID, visible order,
// URL, focus/rename interaction, and layout projection. BrowserShellTabsStore
// owns shared record identity, membership, paths, and names.
export function useShellTabsContextValue(
  providedStore: BrowserShellTabsStore | undefined,
  providedEntry: ShellDocumentEntry | undefined,
): ShellTabsContextValue {
  const store = providedStore ?? getBrowserShellTabsStore()
  const snapshot = useProviderStore(store)
  const entry = useMemo(
    () => providedEntry ?? classifyShellDocumentEntry(),
    [providedEntry],
  )
  const persistedDocumentState = useMemo(
    () => (entry.kind === 'continuation' ? readShellDocumentState() : null),
    [entry],
  )
  const [localOrder, setLocalOrder] = useState<string[]>(() =>
    normalizeLocalOrder([], snapshot.records),
  )
  const [activeTabId, setActiveTabIdState] = useState<string>(() => {
    if (entry.kind === 'handoff' && entry.tabId) {
      return snapshot.records.some((record) => record.id === entry.tabId)
        ? entry.tabId
        : ''
    }
    if (entry.kind === 'continuation') {
      if (
        persistedDocumentState?.incarnation === entry.incarnation &&
        persistedDocumentState?.pendingCreatedTabId &&
        snapshot.records.some(
          (record) => record.id === persistedDocumentState.pendingCreatedTabId,
        )
      ) {
        return persistedDocumentState.pendingCreatedTabId
      }
      if (
        persistedDocumentState?.incarnation === entry.incarnation &&
        persistedDocumentState.activeTabId &&
        snapshot.records.some(
          (record) => record.id === persistedDocumentState.activeTabId,
        )
      ) {
        return persistedDocumentState.activeTabId
      }
      return snapshot.records[0]?.id ?? ''
    }
    return ''
  })
  const [mutationError, setMutationError] =
    useState<BrowserShellTabsStoreError | null>(null)
  const [renamingTabId, setRenamingTabId] = useState<string | null>(null)
  const [activeTabsetPathOpener, setActiveTabsetPathOpener] =
    useState<ActiveTabsetPathOpener | null>(null)
  const initializedRef = useRef(false)
  const activeTabIdRef = useRef(activeTabId)
  activeTabIdRef.current = activeTabId
  const pendingCreatedTabIdRef = useRef<string | null>(
    persistedDocumentState?.incarnation === entry.incarnation
      ? (persistedDocumentState.pendingCreatedTabId ?? null)
      : null,
  )
  const localOrderRef = useRef(localOrder)
  localOrderRef.current = localOrder
  const previousRecordIdsRef = useRef(
    new Set(snapshot.records.map((record) => record.id)),
  )

  const recordsById = useMemo(
    () => new Map(snapshot.records.map((record) => [record.id, record])),
    [snapshot.records],
  )
  const tabs = useMemo(() => {
    const order = normalizeLocalOrder(localOrder, snapshot.records)
    return order
      .map((id) => recordsById.get(id))
      .filter((record): record is BrowserShellTabRecord => record !== undefined)
      .map(recordAsShellTab)
  }, [localOrder, recordsById, snapshot.records])

  useEffect(() => {
    const currentRecordIds = new Set(
      snapshot.records.map((record) => record.id),
    )
    for (const id of previousRecordIdsRef.current) {
      if (!currentRecordIds.has(id)) {
        removeShellTabLocalState(entry.incarnation, id)
      }
    }
    previousRecordIdsRef.current = currentRecordIds
    setLocalOrder((current) => {
      const next = normalizeLocalOrder(current, snapshot.records)
      return current.length === next.length &&
        current.every((id, index) => id === next[index])
        ? current
        : next
    })
    setActiveTabIdState((current) => {
      const pendingCreatedTabId = pendingCreatedTabIdRef.current
      if (pendingCreatedTabId && currentRecordIds.has(pendingCreatedTabId)) {
        return pendingCreatedTabId
      }
      if (current && currentRecordIds.has(current)) return current
      if (!current && (entry.kind === 'fresh' || entry.kind === 'handoff')) {
        return current
      }
      return snapshot.records[0]?.id ?? ''
    })
    setRenamingTabId((current) =>
      current && currentRecordIds.has(current) ? current : null,
    )
  }, [entry, snapshot])

  useEffect(() => {
    if (!activeTabId) return
    const pendingCreatedTabId = pendingCreatedTabIdRef.current
    writeShellDocumentState({
      incarnation: entry.incarnation,
      activeTabId,
      ...(pendingCreatedTabId && pendingCreatedTabId !== activeTabId
        ? { pendingCreatedTabId }
        : {}),
    })
    if (pendingCreatedTabId === activeTabId) {
      pendingCreatedTabIdRef.current = null
    }
  }, [activeTabId, entry.incarnation])

  useEffect(() => {
    if (
      !activeTabId ||
      !Object.prototype.hasOwnProperty.call(entry.params, 'shellTabId')
    ) {
      return
    }
    const navigation = getAppNavigation()
    if (navigation.params.shellTabId === activeTabId) return
    setAppPath(navigation.path, {
      ...navigation.params,
      shellTabId: activeTabId,
    })
  }, [activeTabId, entry.params])
  const reportMutation = useCallback(
    (operation: Promise<unknown>, isCancelled?: () => boolean) => {
      setMutationError(null)
      void operation.catch((error: unknown) => {
        if (isCancelled?.()) return
        const nextError =
          error instanceof BrowserShellTabsStoreError
            ? error
            : new BrowserShellTabsStoreError(
                'storage-write',
                error instanceof Error
                  ? error.message
                  : 'Shell Tab mutation failed.',
              )
        setMutationError(nextError)
        toast.error('Shell tab update failed', {
          description: nextError.message,
        })
      })
    },
    [],
  )
  const stageCreatedTabSelection = useCallback(
    (tabId: string) => {
      pendingCreatedTabIdRef.current = tabId
      writeShellDocumentState({
        incarnation: entry.incarnation,
        activeTabId: activeTabIdRef.current,
        pendingCreatedTabId: tabId,
      })
    },
    [entry.incarnation],
  )
  const clearCreatedTabSelection = useCallback(
    (tabId: string) => {
      if (pendingCreatedTabIdRef.current !== tabId) return
      pendingCreatedTabIdRef.current = null
      writeShellDocumentState({
        incarnation: entry.incarnation,
        activeTabId: activeTabIdRef.current,
      })
    },
    [entry.incarnation],
  )

  useEffect(() => {
    if (initializedRef.current) return
    initializedRef.current = true
    let cancelled = false

    const initialize = async () => {
      clearObsoleteShellTabsState()
      const current = store.read()
      if (entry.kind === 'handoff' && entry.tabId) {
        if (current.records.some((record) => record.id === entry.tabId)) {
          if (!cancelled) setActiveTabIdState(entry.tabId)
          return
        }
      }

      if (entry.kind === 'continuation' && current.records.length > 0) return

      const path = entry.path || '/'
      const id = generateTabId()
      stageCreatedTabSelection(id)
      try {
        await store.create({ id, path, name: getTabNameFromPath(path) })
      } catch (error) {
        clearCreatedTabSelection(id)
        throw error
      }
      if (cancelled) return
      setLocalOrder((order) => insertTabId(order, id, order.at(-1)))
      setActiveTabIdState(id)
      if (Object.prototype.hasOwnProperty.call(entry.params, 'shellTabId')) {
        setAppPath(path, { ...entry.params, shellTabId: id })
      }
    }
    reportMutation(initialize(), () => cancelled)
    return () => {
      cancelled = true
    }
  }, [
    clearCreatedTabSelection,
    entry,
    reportMutation,
    stageCreatedTabSelection,
    store,
  ])

  const setTabs = useCallback<React.Dispatch<React.SetStateAction<ShellTab[]>>>(
    (update) => {
      setLocalOrder((currentOrder) => {
        const currentTabs = currentOrder.flatMap((id) => {
          const record = recordsById.get(id)
          return record ? [recordAsShellTab(record)] : []
        })
        const nextTabs =
          typeof update === 'function' ? update(currentTabs) : update
        const nextIds = nextTabs.flatMap((tab) =>
          recordsById.has(tab.id) ? [tab.id] : [],
        )
        return normalizeLocalOrder(nextIds, snapshot.records)
      })
    },
    [recordsById, snapshot.records],
  )

  const setActiveTabId = useCallback<
    React.Dispatch<React.SetStateAction<string>>
  >(
    (update) => {
      setActiveTabIdState((current) => {
        const next = typeof update === 'function' ? update(current) : update
        return snapshot.records.some((record) => record.id === next)
          ? next
          : current
      })
    },
    [snapshot.records],
  )

  const selectShellTab = useCallback(
    (tabId: string) => {
      if (snapshot.records.some((record) => record.id === tabId)) {
        setActiveTabIdState(tabId)
      }
    },
    [snapshot.records],
  )

  const addShellTab = useCallback(
    (tab: ShellTab, options: AddShellTabOptions = {}) => {
      if (options.select) {
        stageCreatedTabSelection(tab.id)
      }
      const operation = store
        .create({
          id: tab.id,
          path: tab.path,
          name: tab.name,
          customName: tab.customName,
        })
        .then(() => {
          setLocalOrder((order) =>
            insertTabId(order, tab.id, options.afterTabId),
          )
          if (options.select) setActiveTabIdState(tab.id)
          options.onCommitted?.()
        })
        .catch((error: unknown) => {
          if (options.select) {
            clearCreatedTabSelection(tab.id)
          }
          throw error
        })
      reportMutation(operation)
      return tab.id
    },
    [clearCreatedTabSelection, reportMutation, stageCreatedTabSelection, store],
  )

  const openPathInNewTab = useCallback(
    (path: string, options: OpenShellTabOptions = {}) => {
      const select = options.select ?? true
      const existingTab = options.focusExisting
        ? tabs.find((tab) => tab.path === path)
        : undefined
      if (existingTab) {
        if (select) {
          setActiveTabIdState(existingTab.id)
          setAppPath(path)
        }
        return existingTab.id
      }
      const tab: ShellTab = {
        id: generateTabId(),
        name: getTabNameFromPath(path),
        path,
      }
      addShellTab(tab, {
        afterTabId: options.afterTabId,
        select,
        onCommitted: select ? () => setAppPath(path) : undefined,
      })
      return tab.id
    },
    [addShellTab, tabs],
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
    (path: string, options: OpenShellTabOptions = {}) =>
      activeTabsetPathOpener?.(path, options) ??
      openPathInNewTab(path, options),
    [activeTabsetPathOpener, openPathInNewTab],
  )

  const retainShellTabs = useCallback(
    (tabIds: Set<string>, fallbackActiveTabId?: string) => {
      setLocalOrder((order) => order.filter((id) => tabIds.has(id)))
      setActiveTabIdState((current) => {
        if (tabIds.has(current)) return current
        if (fallbackActiveTabId && tabIds.has(fallbackActiveTabId)) {
          return fallbackActiveTabId
        }
        return (
          [...tabIds].find((id) =>
            snapshot.records.some((record) => record.id === id),
          ) ?? ''
        )
      })
    },
    [snapshot.records],
  )

  const closeShellTab = useCallback(
    (tabId: string) => {
      reportMutation(
        store.close(tabId).then(() => {
          removeShellTabLocalState(entry.incarnation, tabId, true)
          const committedRecords = store.getSnapshot().records
          const nextOrder = normalizeLocalOrder(
            localOrderRef.current.filter((id) => id !== tabId),
            committedRecords,
          )
          setLocalOrder(nextOrder)
          setActiveTabIdState((current) => {
            if (
              current !== tabId &&
              committedRecords.some((record) => record.id === current)
            ) {
              return current
            }
            return nextOrder[0] ?? committedRecords[0]?.id ?? ''
          })
        }),
      )
    },
    [entry.incarnation, reportMutation, store],
  )

  const resetShellTabs = useCallback(() => {
    const oldIds = snapshot.records.map((record) => record.id)
    reportMutation(
      store.reset().then((record) => {
        for (const id of oldIds) {
          removeShellTabLocalState(entry.incarnation, id, true)
        }
        setLocalOrder([record.id])
        setActiveTabIdState(record.id)
      }),
    )
  }, [entry.incarnation, reportMutation, snapshot.records, store])

  const updateTabPath = useCallback(
    (tabId: string, path: string, onCommitted?: () => void) => {
      const record = snapshot.records.find(
        (candidate) => candidate.id === tabId,
      )
      if (!record || record.path === path) return Promise.resolve(false)
      const operation = store.updatePath(tabId, path).then((updated) => {
        window.dispatchEvent(
          new CustomEvent(SHELL_TAB_PATH_COMMITTED_EVENT, {
            detail: { tabId, path },
          }),
        )
        onCommitted?.()
        return updated
      })
      reportMutation(operation)
      return operation.then(
        () => true,
        () => false,
      )
    },
    [reportMutation, snapshot.records, store],
  )

  const updateTabAutoName = useCallback(
    (tabId: string, name: string) => {
      const record = snapshot.records.find(
        (candidate) => candidate.id === tabId,
      )
      if (!record || record.name === name) return
      reportMutation(store.updateName(tabId, name))
    },
    [reportMutation, snapshot.records, store],
  )

  const updateTabName = useCallback(
    (tabId: string, customName: string) => {
      reportMutation(store.rename(tabId, customName || undefined))
    },
    [reportMutation, store],
  )

  const startRenaming = useCallback(
    (tabId: string) => setRenamingTabId(tabId),
    [],
  )
  const stopRenaming = useCallback(() => setRenamingTabId(null), [])

  const value = useMemo<ShellTabsContextValue>(
    () => ({
      tabs,
      setTabs,
      activeTabId,
      setActiveTabId,
      documentIncarnation: entry.incarnation,
      openPathInNewTab,
      openPathInActiveTabset,
      registerActiveTabsetPathOpener,
      addShellTab,
      selectShellTab,
      retainShellTabs,
      closeShellTab,
      resetShellTabs,
      updateTabPath,
      updateTabName,
      updateTabAutoName,
      renamingTabId,
      startRenaming,
      stopRenaming,
      mutationError,
    }),
    [
      tabs,
      setTabs,
      activeTabId,
      setActiveTabId,
      entry.incarnation,
      openPathInNewTab,
      openPathInActiveTabset,
      registerActiveTabsetPathOpener,
      addShellTab,
      selectShellTab,
      retainShellTabs,
      closeShellTab,
      resetShellTabs,
      updateTabPath,
      updateTabName,
      updateTabAutoName,
      renamingTabId,
      startRenaming,
      stopRenaming,
      mutationError,
    ],
  )

  return value
}

export function getTabById(
  tabs: ShellTab[],
  tabId: string,
): ShellTab | undefined {
  return tabs.find((tab) => tab.id === tabId)
}

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
  const index = afterTabId ? tabs.findIndex((tab) => tab.id === afterTabId) : -1
  if (index >= 0) {
    const nextTabs = [...tabs]
    nextTabs.splice(index + 1, 0, newTab)
    return { tabs: nextTabs, newTab }
  }
  return { tabs: [...tabs, newTab], newTab }
}

export { DEFAULT_HOME_TAB }
