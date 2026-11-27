import { createContext, use } from 'react'

// TabActiveContext provides whether the current tab is active.
// The app layer populates this. web/ components consume it via
// useIsTabActive(). The default value is true (when no provider is present).
const TabActiveContext = createContext<boolean>(true)

// TabActiveProvider sets the tab-active state for descendant components.
export const TabActiveProvider = TabActiveContext.Provider

// useIsTabActive returns whether the current tab is active.
// The hook returns true if no TabActiveProvider is present (safe default).
export function useIsTabActive(): boolean {
  return use(TabActiveContext)
}
