import type { AddTabRequest, LayoutModel, TabDef } from '../layout.pb.js'
import type { ObjectInfo } from '../../../web/object/object.pb.js'
import {
  ObjectLayout,
  ObjectLayoutTab,
  type ObjectLayout as ObjectLayoutType,
} from './world.pb.js'

export const ObjectLayoutTypeID = 'alpha/object-layout'
export const ObjectLayoutComponentID = 'spacewave.object-layout.viewer'

export interface ObjectLayoutObjectTarget {
  objectInfo?: ObjectInfo
  objectKey?: string
  objectType?: string
  componentID?: string
  path?: string
}

export interface ObjectLayoutTabTarget extends ObjectLayoutObjectTarget {
  id: string
  name: string
  helpText?: string
  enableClose?: boolean
}

export interface ObjectLayoutTabSetTarget {
  id: string
  name?: string
  weight?: number
  tabs: ObjectLayoutTabTarget[]
}

export function createObjectLayoutObjectInfo(
  target: ObjectLayoutObjectTarget,
): ObjectInfo {
  if (target.objectInfo) return target.objectInfo
  return {
    info: {
      case: 'worldObjectInfo',
      value: {
        objectKey: target.objectKey ?? '',
        objectType: target.objectType ?? '',
      },
    },
  }
}

export function createObjectLayoutTabData(
  target: ObjectLayoutObjectTarget,
): Uint8Array {
  return ObjectLayoutTab.toBinary({
    componentId: target.componentID,
    objectInfo: createObjectLayoutObjectInfo(target),
    path: target.path ?? '',
  })
}

export function createObjectLayoutTabDef(
  target: ObjectLayoutTabTarget,
): TabDef {
  return {
    id: target.id,
    name: target.name,
    helpText: target.helpText ?? target.objectKey ?? '',
    enableClose: target.enableClose ?? false,
    data: createObjectLayoutTabData(target),
  }
}

export function createObjectLayoutAddTabRequest(
  target: ObjectLayoutTabTarget & {
    afterTabId?: string
    select?: boolean
    tabSetId?: string
  },
): AddTabRequest {
  return {
    tabSetId: target.tabSetId,
    afterTabId: target.afterTabId,
    select: target.select ?? true,
    tab: createObjectLayoutTabDef(target),
  }
}

export function createSingleRowObjectLayoutModel(
  tabSets: ObjectLayoutTabSetTarget[],
  rootId = 'root',
): LayoutModel {
  return {
    layout: {
      id: rootId,
      children: tabSets.map((tabSet) => ({
        node: {
          case: 'tabSet',
          value: {
            id: tabSet.id,
            name: tabSet.name ?? '',
            weight: tabSet.weight ?? 0,
            children: tabSet.tabs.map(createObjectLayoutTabDef),
          },
        },
      })),
    },
  }
}

export function createSingleRowObjectLayout(
  tabSets: ObjectLayoutTabSetTarget[],
  rootId?: string,
): ObjectLayoutType {
  return {
    layoutModel: createSingleRowObjectLayoutModel(tabSets, rootId),
  }
}

export function serializeObjectLayout(layout: ObjectLayoutType): Uint8Array {
  return ObjectLayout.toBinary(layout)
}
