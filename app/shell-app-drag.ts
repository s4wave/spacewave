import type { DragEvent as ReactDragEvent } from 'react'
import type { IJsonTabNode, Node } from '@aptre/flex-layout'
import { readAppDragEnvelopeWithActiveFallback } from '@s4wave/web/dnd/app-drag.js'
import {
  generateTabId,
  getTabNameFromPath,
  type ShellTab,
} from './shell-tab.js'

export interface ShellExternalDragResult {
  json: IJsonTabNode
  onDrop: (node?: Node) => void
}

export function buildShellExternalDrag(
  event: ReactDragEvent<HTMLElement>,
  onAddTabs: (tabs: ShellTab[], node?: Node) => void,
): ShellExternalDragResult | undefined {
  const envelope = readAppDragEnvelopeWithActiveFallback(event.dataTransfer)
  const tabs =
    envelope?.items.flatMap((item) => {
      const capability = item.capabilities.find(
        (cap) =>
          cap.kind === 'openable' &&
          cap.value.case === 'object' &&
          !!cap.value.value.routePath,
      )
      if (!capability || capability.kind !== 'openable') return []

      const routePath = capability.value.value.routePath
      if (!routePath) return []

      return [
        {
          id: generateTabId(),
          name: item.label || getTabNameFromPath(routePath),
          path: routePath,
        },
      ]
    }) ?? []
  const firstTab = tabs[0]
  if (!firstTab) return undefined

  return {
    json: {
      type: 'tab',
      id: firstTab.id,
      name: firstTab.name,
      component: 'shell-content',
    },
    onDrop: (node) => {
      onAddTabs(
        node ? [{ ...firstTab, id: node.getId() }, ...tabs.slice(1)] : tabs,
        node,
      )
    },
  }
}
