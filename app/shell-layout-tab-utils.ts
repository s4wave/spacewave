import { Actions, DockLocation, Model } from '@aptre/flex-layout'

import {
  DEFAULT_HOME_TAB,
  getSessionPathFromPath,
  type ShellTab,
  generateTabId,
  getTabDisplayName,
  getTabNameFromPath,
} from '@s4wave/app/shell-tab.js'

// addShellModelTab adds a shell tab node to a FlexLayout model.
export function addShellModelTab(
  model: Model,
  tabsetId: string,
  tab: ShellTab,
  component: string,
): void {
  model.doAction(
    Actions.addNode(
      {
        type: 'tab',
        id: tab.id,
        name: getTabDisplayName(tab),
        component,
      },
      tabsetId,
      DockLocation.CENTER,
      -1,
    ),
  )
}

// addAndSelectShellModelTab adds a shell tab node and makes it active.
export function addAndSelectShellModelTab(
  model: Model,
  tabsetId: string,
  tab: ShellTab,
  component: string,
): void {
  addShellModelTab(model, tabsetId, tab, component)
  model.doAction(Actions.selectTab(tab.id))
}

// buildPathTab builds a tab for a specific shell path.
export function buildPathTab(path: string): ShellTab {
  return {
    id: generateTabId(),
    name: getTabNameFromPath(path),
    path,
  }
}

// buildContextualShellTab builds a new tab based on the current shell path.
export function buildContextualShellTab(path: string | undefined): ShellTab {
  const sessionPath = path ? getSessionPathFromPath(path) : null
  return {
    id: generateTabId(),
    name: sessionPath ? 'Session' : DEFAULT_HOME_TAB.name,
    path: sessionPath ?? DEFAULT_HOME_TAB.path,
  }
}

// findShellTab returns the tab with the given ID if it exists.
export function findShellTab(
  tabs: ShellTab[],
  tabId: string | null | undefined,
): ShellTab | undefined {
  if (!tabId) return undefined
  return tabs.find((tab) => tab.id === tabId)
}

// cloneShellTab copies a shell tab into a new tab ID.
export function cloneShellTab(tab: ShellTab): ShellTab {
  return {
    id: generateTabId(),
    name: tab.name,
    path: tab.path,
    customName: tab.customName,
  }
}

// countShellModelTabs returns the number of tab nodes in a layout model.
export function countShellModelTabs(model: Model | null): number {
  if (!model) return 0

  let count = 0
  model.visitNodes((node) => {
    if (node.getType() === 'tab') {
      count++
    }
  })
  return count
}

// getShellTabsetId returns the parent tabset ID for a tab node if present.
export function getShellTabsetId(
  model: Model,
  tabId: string | null | undefined,
): string | null {
  if (!tabId) return null

  const node = model.getNodeById(tabId)
  if (!node || node.getType() !== 'tab') return null

  const parent = node.getParent()
  if (!parent || parent.getType() !== 'tabset') return null

  return parent.getId()
}
