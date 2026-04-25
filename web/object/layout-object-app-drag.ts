import type { DragEvent as ReactDragEvent } from 'react'
import type { IJsonTabNode } from '@aptre/flex-layout'
import { ObjectLayoutTab } from '@s4wave/sdk/layout/world/world.pb.js'
import { readAppDragEnvelopeWithActiveFallback } from '@s4wave/web/dnd/app-drag.js'

export interface ObjectLayoutExternalDragResult {
  json: IJsonTabNode
}

function generateObjectLayoutTabId(): string {
  return `tab-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
}

export function buildObjectLayoutExternalDrag(
  event: ReactDragEvent<HTMLElement>,
): ObjectLayoutExternalDragResult | undefined {
  const envelope = readAppDragEnvelopeWithActiveFallback(event.dataTransfer)
  const item = envelope?.items.find((item) =>
    item.capabilities.some(
      (cap) => cap.kind === 'openable' && cap.value.case === 'object',
    ),
  )
  if (!item) return undefined

  const capability = item.capabilities.find(
    (cap) => cap.kind === 'openable' && cap.value.case === 'object',
  )
  if (!capability || capability.kind !== 'openable') return undefined

  const openable = capability.value.value
  const tabData = ObjectLayoutTab.toBinary({
    componentId: openable.componentId,
    objectInfo: openable.objectInfo,
    path: openable.path,
  })

  return {
    json: {
      type: 'tab',
      id: generateObjectLayoutTabId(),
      name: item.label || 'Object',
      component: 'tab-content',
      config: tabData,
    },
  }
}
