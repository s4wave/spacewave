import { createContext, useContext } from 'react'

// TabActiveContext provides whether the current tab is active.
// App layer populates this; web/ components consume it via useIsTabActive().
// Returns true by default (when no provider is present).
const TabActiveContext = createContext<boolean>(true)

// TabActiveProvider sets the tab-active state for descendant components.
export const TabActiveProvider = TabActiveContext.Provider

// useIsTabActive returns whether the current tab is active.
// Returns true if no TabActiveProvider is present (safe default).
export function useIsTabActive(): boolean {
  return useContext(TabActiveContext)
}
