import { createContext, use } from 'react'
import type {
  AddTabRequest,
  AddTabResponse,
  NavigateTabResponse,
  ReplaceTabRequest,
  ReplaceTabResponse,
} from '@s4wave/sdk/layout/layout.pb.js'

export type ReplaceCurrentTabRequest = Omit<ReplaceTabRequest, 'tabId'>

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
  // replaceTab replaces the current tab payload without moving the tab.
  replaceTab?: (
    request: ReplaceCurrentTabRequest,
  ) => Promise<ReplaceTabResponse>
  // isObjectLayout is true when the context is provided by an ObjectLayout tab.
  isObjectLayout?: boolean
}

const TabContext = createContext<TabContextValue | null>(null)

// TabContextProvider provides tab operations to descendant components.
export const TabContextProvider = TabContext.Provider

// useTabContext returns the tab context, or null if not inside a tab.
export function useTabContext(): TabContextValue | null {
  return use(TabContext)
}

// useTabId returns the current tab ID from TabContext.
export function useTabId(): string | null {
  return use(TabContext)?.tabId ?? null
}
