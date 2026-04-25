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
  onAddTab: (tab: ShellTab) => void,
): ShellExternalDragResult | undefined {
  const envelope = readAppDragEnvelopeWithActiveFallback(event.dataTransfer)
  const item = envelope?.items.find((item) =>
    item.capabilities.some(
      (cap) =>
        cap.kind === 'openable' &&
        cap.value.case === 'object' &&
        !!cap.value.value.routePath,
    ),
  )
  if (!item) return undefined

  const capability = item.capabilities.find(
    (cap) =>
      cap.kind === 'openable' &&
      cap.value.case === 'object' &&
      !!cap.value.value.routePath,
  )
  if (!capability || capability.kind !== 'openable') return undefined

  const routePath = capability.value.value.routePath
  if (!routePath) return undefined

  const tabId = generateTabId()
  const name = item.label || getTabNameFromPath(routePath)
  return {
    json: {
      type: 'tab',
      id: tabId,
      name,
      component: 'shell-content',
    },
    onDrop: (node) => {
      onAddTab({
        id: node?.getId() ?? tabId,
        name,
        path: routePath,
      })
    },
  }
}
