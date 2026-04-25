import { createContext, useContext } from 'react'
import type {
  AddTabRequest,
  AddTabResponse,
  NavigateTabResponse,
} from '@s4wave/sdk/layout/layout.pb.js'

// TabContextValue provides tab operations to descendant components.
// Both BaseLayout (FlexLayout) and ShellTabs provide this context
// with their own implementations.
export interface TabContextValue {
  // tabId is the unique identifier of the current tab.
  tabId: string
  // addTab adds a new tab to the layout.
  addTab: (request: AddTabRequest) => Promise<AddTabResponse>
  // navigateTab navigates the current tab to a new path.
  navigateTab: (path: string) => Promise<NavigateTabResponse>
}

const TabContext = createContext<TabContextValue | null>(null)

// TabContextProvider provides tab operations to descendant components.
export const TabContextProvider = TabContext.Provider

// useTabContext returns the tab context, or null if not inside a tab.
export function useTabContext(): TabContextValue | null {
  return useContext(TabContext)
}

// useTabId returns the current tab ID from TabContext.
export function useTabId(): string | null {
  return useContext(TabContext)?.tabId ?? null
}
