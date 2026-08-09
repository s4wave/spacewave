import { useCallback, type DragEvent as ReactDragEvent } from 'react'
import { Actions, type Model } from '@aptre/flex-layout'

import { setAppPath } from '@s4wave/web/router/app-path.js'

import type { ShellTabsContextValue } from './ShellTabContext.js'
import { buildShellExternalDrag } from './shell-app-drag.js'
import {
  addAndSelectShellModelTab,
  addShellModelTab,
  getShellTabsetId,
} from './shell-layout-tab-utils.js'

interface ShellExternalAppDragOptions {
  addShellTab: ShellTabsContextValue['addShellTab']
  isGridMode: () => boolean
  markShellEngaged: () => void
  model: Model
}

export function useShellExternalAppDrag({
  addShellTab,
  isGridMode,
  markShellEngaged,
  model,
}: ShellExternalAppDragOptions) {
  return useCallback(
    (event: ReactDragEvent<HTMLElement>) =>
      buildShellExternalDrag(event, (draggedTabs, droppedNode) => {
        const [firstTab, ...remainingTabs] = draggedTabs
        if (!firstTab) return

        const droppedTabId = droppedNode?.getId()
        const droppedTab = droppedTabId ? model.getNodeById(droppedTabId) : null
        const tabsetId =
          droppedTabId && droppedTab
            ? (getShellTabsetId(model, droppedTabId) ?? 'shell-tabset')
            : 'shell-tabset'
        const activeTab =
          droppedTabId && droppedTab
            ? { ...firstTab, id: droppedTabId }
            : firstTab

        const commitFirst = () => {
          if (!droppedTab) {
            addAndSelectShellModelTab(
              model,
              tabsetId,
              activeTab,
              'shell-content',
            )
          }
          if (!isGridMode()) setAppPath(activeTab.path)
          model.doAction(Actions.selectTab(activeTab.id))
          markShellEngaged()
        }
        addShellTab(activeTab, { select: true, onCommitted: commitFirst })
        for (const tab of remainingTabs) {
          addShellTab(tab, {
            onCommitted: () =>
              addShellModelTab(model, tabsetId, tab, 'shell-content'),
          })
        }
      }),
    [addShellTab, isGridMode, markShellEngaged, model],
  )
}
