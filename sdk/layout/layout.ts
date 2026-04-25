import merge from 'deepmerge'
import {
  Actions,
  IJsonModel,
  IJsonRowNode,
  IJsonTabSetNode,
  IJsonBorderNode,
  IBorderLocation,
  IJsonTabNode,
  Model,
} from '@aptre/flex-layout'
import isDeepEqual from 'lodash.isequal'

import {
  BorderDef,
  BorderLocation,
  LayoutModel,
  RowDef,
  RowOrTabSetDef,
  TabDef,
  TabSetDef,
} from './layout.pb.js'

export function protoToBorderLocation(loc: BorderLocation): IBorderLocation {
  switch (loc) {
    case BorderLocation.BorderLocation_BOTTOM:
      return 'bottom'
    case BorderLocation.BorderLocation_LEFT:
      return 'left'
    case BorderLocation.BorderLocation_RIGHT:
      return 'right'
    default:
    case BorderLocation.BorderLocation_TOP:
      return 'top'
  }
}

export function borderLocationToProto(loc: IBorderLocation): BorderLocation {
  switch (loc) {
    case 'bottom':
      return BorderLocation.BorderLocation_BOTTOM
    case 'left':
      return BorderLocation.BorderLocation_LEFT
    case 'right':
      return BorderLocation.BorderLocation_RIGHT
    case 'top':
      return BorderLocation.BorderLocation_TOP
    default:
      return 0
  }
}

export function protoToTabNode(
  tabDef: TabDef,
  tabDataMap: TabDataMap,
): IJsonTabNode {
  tabDataMap[tabDef.id ?? ''] = tabDef.data ?? new Uint8Array()
  return {
    type: 'tab',
    id: tabDef.id,
    name: tabDef.name || undefined,
    helpText: tabDef.helpText || undefined,
    enableClose: tabDef.enableClose || false,
  }
}

export function tabNodeToProto(
  node: IJsonTabNode,
  tabDataMap: TabDataMap,
): TabDef {
  let data: Uint8Array | undefined = undefined
  if (node.id) {
    data = tabDataMap[node.id]
  }
  if (data == null && node.config instanceof Uint8Array) {
    data = node.config
  }
  return {
    id: node.id ?? '',
    name: node.name ?? '',
    helpText: node.helpText ?? '',
    enableClose: node.enableClose || false,
    data: data || new Uint8Array(0),
  }
}

export function protoToTabNodeList(
  def: TabDef[],
  tabDataMap: TabDataMap,
): IJsonTabNode[] {
  return def?.length ? def.map((def) => protoToTabNode(def, tabDataMap)) : []
}

export function tabNodeListToProto(
  tabNodeList: IJsonTabNode[],
  tabDataMap: TabDataMap,
): TabDef[] {
  return tabNodeList?.length ?
      tabNodeList.map((def) => tabNodeToProto(def, tabDataMap))
    : []
}

export function protoToBorderNode(
  borderDef: BorderDef,
  tabDataMap: TabDataMap,
): IJsonBorderNode {
  return {
    type: 'border',
    location: protoToBorderLocation(
      borderDef.borderLocation ?? BorderLocation.BorderLocation_LEFT,
    ),
    children:
      borderDef.children?.map((child) => protoToTabNode(child, tabDataMap)) ??
      [],
    selected: borderDef.selected ?? -1,
    show: !(borderDef.hide ?? false),
  }
}

export function borderNodeToProto(
  node: IJsonBorderNode,
  tabDataMap: TabDataMap,
): BorderDef {
  return {
    borderLocation: borderLocationToProto(node.location),
    children: tabNodeListToProto(node.children, tabDataMap),
    selected: node.selected ?? -1,
    hide: !(node.show ?? true),
  }
}

export function protoToRowNode(
  rowDef: RowDef,
  tabDataMap: TabDataMap,
): IJsonRowNode {
  return {
    type: 'row',
    id: rowDef?.id,
    children: protoToRowOrTabSetNodeList(rowDef?.children, tabDataMap),
    weight: rowDef?.weight || undefined,
  }
}

export function rowNodeToProto(
  rowNode: IJsonRowNode,
  tabDataMap: TabDataMap,
  localState?: ILocalState,
): RowDef {
  return {
    id: rowNode?.id,
    children: rowOrTabSetNodeListToProto(
      rowNode?.children,
      tabDataMap,
      localState,
    ),
    weight: rowNode?.weight || 0,
  }
}

export function protoToTabSetNode(
  tabSetDef: TabSetDef,
  tabDataMap: TabDataMap,
): IJsonTabSetNode {
  return {
    type: 'tabset',
    id: tabSetDef?.id || undefined,
    name: tabSetDef?.name || undefined,
    weight: tabSetDef?.weight || undefined,
    children:
      tabSetDef?.children?.length ?
        protoToTabNodeList(tabSetDef?.children, tabDataMap)
      : [],
  }
}

export function tabSetNodeToProto(
  tabSetNode: IJsonTabSetNode,
  tabDataMap: TabDataMap,
  localState?: ILocalState,
): TabSetDef {
  const id = tabSetNode?.id
  if (localState && id) {
    if (tabSetNode.active) {
      localState.activeTabSet = id
    }
    const selected = tabSetNode.selected
    if (
      typeof selected === 'number' &&
      selected >= 0 &&
      selected < tabSetNode.children.length
    ) {
      localState.tabSetSelected[id] = tabSetNode.children[selected].id || ''
    } else {
      delete localState.tabSetSelected[id]
    }
    if (tabSetNode.maximized) {
      localState.maximizedTab = id
    }
  }

  return {
    id: id ?? '',
    name: tabSetNode?.name ?? '',
    weight: tabSetNode?.weight || 0,
    children: tabNodeListToProto(tabSetNode?.children || [], tabDataMap),
  }
}

export function protoToRowOrTabSetNode(
  def: RowOrTabSetDef,
  tabDataMap: TabDataMap,
): IJsonRowNode | IJsonTabSetNode {
  const node = def.node
  switch (node?.case) {
    case 'row':
      return protoToRowNode(node.value, tabDataMap)
    case 'tabSet':
      return protoToTabSetNode(node.value, tabDataMap)
    default: {
      const def: IJsonTabSetNode = { type: 'tabset', children: [] }
      return def
    }
  }
}

export function rowOrTabSetNodeToProto(
  def: IJsonRowNode | IJsonTabSetNode,
  tabDataMap: TabDataMap,
  localState?: ILocalState,
): RowOrTabSetDef {
  switch (def.type) {
    case 'row':
      return {
        node: {
          case: 'row',
          value: rowNodeToProto(def, tabDataMap, localState),
        },
      }
    default:
      return {
        node: {
          case: 'tabSet',
          value: tabSetNodeToProto(def, tabDataMap, localState),
        },
      }
  }
}

export function protoToRowOrTabSetNodeList(
  def: RowOrTabSetDef[] | undefined,
  tabDataMap: TabDataMap,
): (IJsonRowNode | IJsonTabSetNode)[] {
  return def?.length ?
      def.map((def) => protoToRowOrTabSetNode(def, tabDataMap))
    : []
}

export function rowOrTabSetNodeListToProto(
  def: (IJsonRowNode | IJsonTabSetNode)[] | undefined,
  tabDataMap: TabDataMap,
  localState?: ILocalState,
): RowOrTabSetDef[] {
  return def?.length ?
      def.map((def) => rowOrTabSetNodeToProto(def, tabDataMap, localState))
    : []
}

export function protoToBorderNodeList(
  def: BorderDef[] | undefined,
  tabDataMap: TabDataMap,
): IJsonBorderNode[] {
  return def?.length ? def.map((def) => protoToBorderNode(def, tabDataMap)) : []
}

export function layoutModelToJsonModel(
  modelBase: IJsonModel,
  tabDataMap: TabDataMap,
  layoutModel?: LayoutModel,
): IJsonModel {
  return merge(modelBase, {
    borders: protoToBorderNodeList(layoutModel?.borders, tabDataMap),
    layout:
      layoutModel?.layout ?
        protoToRowNode(layoutModel.layout, tabDataMap)
      : undefined,
  })
}

// SelectedTabMap is a map of tabset ID to selected tab ID.
export type SelectedTabMap = { [tabSetId: string]: string }

// ILocalState is local layout state (not persisted).
export interface ILocalState {
  // activeTabSet is the active tab set ID.
  activeTabSet?: string
  // tabSetSelected is the selected tab for each tabset.
  tabSetSelected: SelectedTabMap
  // maximizedTab is the maximized tab ID, if any.
  maximizedTab?: string
}

// TabDataMap maps tab ID to tab data.
export type TabDataMap = Record<string, Uint8Array>

export function cloneLocalState(localState?: ILocalState): ILocalState {
  let result: ILocalState = { tabSetSelected: {} }
  if (localState) {
    result = merge(result, localState)
  }
  return result
}

export function jsonModelToLayoutModel(
  model: IJsonModel,
  tabDataMap: TabDataMap,
  localState?: ILocalState,
): LayoutModel {
  return {
    borders:
      model.borders?.map((border) => borderNodeToProto(border, tabDataMap)) ??
      [],
    layout: rowNodeToProto(model.layout, tabDataMap, localState),
  }
}

export function compareLocalState(ls1: ILocalState, ls2: ILocalState) {
  return (
    ls1.activeTabSet === ls2.activeTabSet &&
    ls1.maximizedTab === ls2.maximizedTab &&
    isDeepEqual(ls1.tabSetSelected, ls2.tabSetSelected)
  )
}

export function applyLocalStateToModel(model: Model, localState: ILocalState) {
  const tabSetSelected = localState.tabSetSelected
  for (const tabSetID of Object.keys(tabSetSelected)) {
    const tabSetNode = model.getNodeById(tabSetID)
    if (!tabSetNode) {
      delete tabSetSelected[tabSetID]
      continue
    }
    model.doAction(Actions.selectTab(tabSetSelected[tabSetID]))
  }
  if (localState.activeTabSet) {
    model.doAction(Actions.setActiveTabset(localState.activeTabSet))
  }
  if (localState.maximizedTab) {
    model.doAction(Actions.maximizeToggle(localState.maximizedTab))
  }
}
