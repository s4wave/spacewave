import { useCallback } from 'react'
import type { Model } from '@aptre/flex-layout'

import type { ShellTab } from '@s4wave/app/shell-tab.js'

import type { ShellTabsContextValue } from './ShellTabContext.js'
import {
  addAndSelectShellModelTab,
  buildContextualShellTab,
  cloneShellTab,
  findShellTab,
  getShellTabsetId,
} from './shell-layout-tab-utils.js'
import { openShellTabInNewTab } from './shell-popout.js'

interface ShellTabActionsOptions {
  activeTabId: string
  addShellTab: ShellTabsContextValue['addShellTab']
  closeShellTab: ShellTabsContextValue['closeShellTab']
  model: Model
  tabs: ShellTab[]
}

export function useShellTabActions({
  activeTabId,
  addShellTab,
  closeShellTab,
  model,
  tabs,
}: ShellTabActionsOptions) {
  const appendAndSelectTab = useCallback(
    (tab: ShellTab, tabsetId = 'shell-tabset') => {
      addShellTab(tab, {
        select: true,
        onCommitted: () =>
          addAndSelectShellModelTab(model, tabsetId, tab, 'shell-content'),
      })
    },
    [addShellTab, model],
  )

  const newTabAt = useCallback(
    (tabId: string) => {
      const sourceTab = findShellTab(tabs, tabId)
      const tabsetId = getShellTabsetId(model, tabId) ?? 'shell-tabset'
      appendAndSelectTab(buildContextualShellTab(sourceTab?.path), tabsetId)
    },
    [appendAndSelectTab, model, tabs],
  )

  const newTab = useCallback(
    () => newTabAt(activeTabId),
    [activeTabId, newTabAt],
  )
  const popoutTab = useCallback(() => {
    const activeTab = findShellTab(tabs, activeTabId)
    if (activeTab) openShellTabInNewTab(activeTab.path, activeTab.id)
  }, [activeTabId, tabs])
  const closeTab = useCallback(() => {
    if (tabs.length > 1) closeShellTab(activeTabId)
  }, [activeTabId, closeShellTab, tabs.length])
  const closeTabById = useCallback(
    (tabId: string) => {
      if (tabs.length > 1) closeShellTab(tabId)
    },
    [closeShellTab, tabs.length],
  )
  const duplicateTab = useCallback(
    (tabId: string) => {
      const tab = findShellTab(tabs, tabId)
      if (!tab) return
      const tabsetId = getShellTabsetId(model, tabId) ?? 'shell-tabset'
      appendAndSelectTab(cloneShellTab(tab), tabsetId)
    },
    [appendAndSelectTab, model, tabs],
  )
  const closeOtherTabs = useCallback(
    (keepTabId: string) => {
      for (const tab of tabs) {
        if (tab.id !== keepTabId) closeShellTab(tab.id)
      }
    },
    [closeShellTab, tabs],
  )
  const popoutTabById = useCallback(
    (tabId: string) => {
      const tab = findShellTab(tabs, tabId)
      if (tab) openShellTabInNewTab(tab.path, tab.id)
    },
    [tabs],
  )

  return {
    closeOtherTabs,
    closeTab,
    closeTabById,
    duplicateTab,
    newTab,
    newTabAt,
    popoutTab,
    popoutTabById,
  }
}
