import { describe, expect, it } from 'vitest'

import { jsonModelToLayoutModel, layoutModelToJsonModel } from '../layout.js'
import { ObjectLayoutTab } from './world.pb.js'
import {
  createObjectLayoutAddTabRequest,
  createObjectLayoutReplaceTabRequest,
  createObjectLayoutTabDef,
  createSingleRowObjectLayout,
  serializeObjectLayout,
} from './object-layout.js'
import { ObjectLayout } from './world.pb.js'

describe('object layout helpers', () => {
  it('stores object, component, and path in the persisted tab payload', () => {
    const tab = createObjectLayoutTabDef({
      id: 'decision-proof',
      name: 'Proof',
      objectKey: 'gizmo/decision/release',
      objectType: 'gizmo/decision',
      componentID: 'gizmo.decision',
      path: '/proof',
      enableClose: true,
    })

    expect(tab).toMatchObject({
      id: 'decision-proof',
      name: 'Proof',
      helpText: 'gizmo/decision/release',
      enableClose: true,
    })
    const payload = ObjectLayoutTab.fromBinary(tab.data ?? new Uint8Array())
    expect(payload.componentId).toBe('gizmo.decision')
    expect(payload.path).toBe('/proof')
    expect(payload.objectInfo?.info).toMatchObject({
      case: 'worldObjectInfo',
      value: {
        objectKey: 'gizmo/decision/release',
        objectType: 'gizmo/decision',
      },
    })
  })

  it('builds ObjectLayout add-tab requests with the same payload contract', () => {
    const request = createObjectLayoutAddTabRequest({
      tabSetId: 'detail',
      afterTabId: 'home',
      id: 'chat',
      name: 'Chat',
      objectKey: 'gizmo/llm-session/live',
      objectType: 'gizmo/llm-session',
      componentID: 'gizmo.llm-session',
      path: '/chat',
    })

    expect(request).toMatchObject({
      tabSetId: 'detail',
      afterTabId: 'home',
      select: true,
      tab: {
        id: 'chat',
        name: 'Chat',
      },
    })
    const payload = ObjectLayoutTab.fromBinary(
      request.tab?.data ?? new Uint8Array(),
    )
    expect(payload.componentId).toBe('gizmo.llm-session')
    expect(payload.path).toBe('/chat')
  })

  it('builds ObjectLayout replace-tab requests that clear or preserve component selection explicitly', () => {
    const ordinary = createObjectLayoutReplaceTabRequest({
      tabId: 'current',
      name: 'Files',
      objectKey: 'files',
      objectType: 'unixfs/fs-node',
      path: '/docs',
    })
    expect(ordinary.tabId).toBe('current')
    expect(ordinary.tab?.id).toBe('current')
    expect(ordinary.tab?.enableClose).toBeUndefined()
    const ordinaryPayload = ObjectLayoutTab.fromBinary(
      ordinary.tab?.data ?? new Uint8Array(),
    )
    expect(ordinaryPayload.componentId).toBeUndefined()
    expect(ordinaryPayload.path).toBe('/docs')
    expect(ordinaryPayload.objectInfo?.info).toMatchObject({
      case: 'worldObjectInfo',
      value: {
        objectKey: 'files',
        objectType: 'unixfs/fs-node',
      },
    })

    const explicit = createObjectLayoutReplaceTabRequest({
      tabId: 'current',
      name: 'Files',
      objectKey: 'files',
      objectType: 'unixfs/fs-node',
      componentID: 'spacewave.files.preview',
      path: '/docs',
    })
    const explicitPayload = ObjectLayoutTab.fromBinary(
      explicit.tab?.data ?? new Uint8Array(),
    )
    expect(explicitPayload.componentId).toBe('spacewave.files.preview')
  })

  it('round-trips authored dashboard tabs through layout serialization', () => {
    const layout = createSingleRowObjectLayout([
      {
        id: 'focus',
        weight: 45,
        tabs: [
          {
            id: 'home',
            name: 'Gizmo Home',
            objectKey: 'gizmo/operator-home',
            objectType: 'gizmo/operator-home',
            componentID: 'gizmo.operator-home',
          },
        ],
      },
      {
        id: 'detail',
        weight: 55,
        tabs: [
          {
            id: 'internals',
            name: 'Internals',
            objectKey: 'gizmo/decision/release',
            objectType: 'gizmo/decision',
            componentID: 'spacewave.debug.viewer',
            path: '/internals',
          },
        ],
      },
    ])

    const decoded = ObjectLayout.fromBinary(serializeObjectLayout(layout))
    const tabDataMap: Record<string, Uint8Array> = {}
    const json = layoutModelToJsonModel(
      { global: {}, borders: [], layout: { type: 'row', children: [] } },
      tabDataMap,
      decoded.layoutModel,
    )
    const nextLocal = { tabSetSelected: {} }
    const roundTripped = jsonModelToLayoutModel(json, tabDataMap, nextLocal)
    const detail =
      roundTripped.layout?.children?.[1]?.node?.case === 'tabSet'
        ? roundTripped.layout.children[1].node.value
        : undefined
    const internals = detail?.children?.[0]
    const payload = ObjectLayoutTab.fromBinary(
      internals?.data ?? new Uint8Array(),
    )

    expect(roundTripped.layout?.children).toHaveLength(2)
    expect(internals?.id).toBe('internals')
    expect(payload.componentId).toBe('spacewave.debug.viewer')
    expect(payload.path).toBe('/internals')
  })
})
