import { useMemo, useState, type ReactNode } from 'react'

import { TabActiveProvider } from '@s4wave/web/contexts/TabActiveContext.js'
import {
  StateNamespaceProvider,
  StorageAtom,
  type Atom,
  type StateType,
  type Storage as StateStorage,
} from '@s4wave/web/state/index.js'

import {
  ShellTabContext,
  useIsTabActive,
  useShellTabs,
  type ShellTabContextValue,
} from './ShellTabContext.js'
import { shellTabStateStorageKey } from './ShellDocumentState.js'

const sessionStorageBackend: StateStorage = {
  getItem: (key) =>
    typeof sessionStorage === 'undefined' ? null : sessionStorage.getItem(key),
  setItem: (key, value) => {
    if (typeof sessionStorage !== 'undefined')
      sessionStorage.setItem(key, value)
  },
  removeItem: (key) => {
    if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem(key)
  },
}

export function ShellTabStateProvider({
  tabId,
  children,
}: {
  tabId: string
  children: ReactNode
}) {
  const { documentIncarnation } = useShellTabs()
  const [atomCache] = useState(() => new Map<string, Atom<StateType>>())
  const tabStateAtom = useMemo(() => {
    const key = shellTabStateStorageKey(documentIncarnation, tabId)
    const cached = atomCache.get(key)
    if (cached) return cached
    const atom = new StorageAtom<StateType>(sessionStorageBackend, key, {})
    atomCache.set(key, atom)
    return atom
  }, [atomCache, documentIncarnation, tabId])
  const contextValue = useMemo<ShellTabContextValue>(() => ({ tabId }), [tabId])
  return (
    <ShellTabContext.Provider value={contextValue}>
      <TabActiveBridge>
        <StateNamespaceProvider
          rootAtom={tabStateAtom}
          inheritStateAtomAccessor={false}
        >
          {children}
        </StateNamespaceProvider>
      </TabActiveBridge>
    </ShellTabContext.Provider>
  )
}

function TabActiveBridge({ children }: { children: ReactNode }) {
  const isActive = useIsTabActive()
  return <TabActiveProvider value={isActive}>{children}</TabActiveProvider>
}
