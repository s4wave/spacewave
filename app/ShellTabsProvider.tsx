import type { ReactNode } from 'react'

import type { BrowserShellTabsStore } from './BrowserShellTabsStore.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'
import {
  ShellTabsContext,
  useShellTabsContextValue,
} from './ShellTabContext.js'

export interface ShellTabsProviderProps {
  children: ReactNode
  store?: BrowserShellTabsStore
  entry?: ShellDocumentEntry
}

// ShellTabsProvider publishes the document-local Shell Tab owner.
export function ShellTabsProvider({
  children,
  store,
  entry,
}: ShellTabsProviderProps) {
  const value = useShellTabsContextValue(store, entry)
  return (
    <ShellTabsContext.Provider value={value}>
      {children}
    </ShellTabsContext.Provider>
  )
}
