import { Model, IJsonModel, Actions } from '@aptre/flex-layout'

import {
  LayoutModel,
  LayoutSnapshot,
  LayoutLocalState,
} from '@s4wave/sdk/layout/layout.pb.js'
import {
  jsonModelToLayoutModel,
  layoutModelToJsonModel,
  type TabDataMap,
} from '@s4wave/sdk/layout/layout.js'
import { BASE_MODEL } from '@s4wave/web/layout/layout.js'

// SHELL_GRID_BASE_MODEL is the base FlexLayout configuration for grid mode.
export const SHELL_GRID_BASE_MODEL: IJsonModel = {
  ...BASE_MODEL,
  global: {
    ...BASE_MODEL.global,
    tabEnableClose: false,
    tabSetEnableMaximize: true,
    tabSetEnableDivide: true,
    tabSetEnableDeleteWhenEmpty: true,
    splitterSize: 4,
    splitterExtra: 0,
    enableEdgeDock: true,
  },
  layout: { type: 'row', weight: 100, children: [] },
}

// hasGridLayout checks if the model has multiple tabsets (splits).
// Returns true if the layout has been split, false if it's a single tabset.
export function hasGridLayout(model: Model): boolean {
  let tabsetCount = 0

  model.visitNodes((node) => {
    if (node.getType() === 'tabset') {
      tabsetCount++
    }
  })

  // Grid mode only if there are multiple tabsets
  return tabsetCount > 1
}

// getTabIdsFromModel extracts all tab IDs from the model.
export function getTabIdsFromModel(model: Model): string[] {
  const ids: string[] = []
  model.visitNodes((node) => {
    if (node.getType() === 'tab') {
      ids.push(node.getId())
    }
  })
  return ids
}

// getSelectedTabId returns the ID of the selected tab in the active tabset.
export function getSelectedTabId(model: Model): string | null {
  let selectedId: string | null = null

  model.visitNodes((node) => {
    if (node.getType() === 'tabset') {
      const tabset = node as import('@aptre/flex-layout').TabSetNode
      if (tabset.isActive()) {
        const selected = tabset.getSelectedNode()
        if (selected) {
          selectedId = selected.getId()
        }
      }
    }
  })

  // Fallback: if no active tabset found, get the first tabset's selected tab
  if (!selectedId) {
    model.visitNodes((node) => {
      if (node.getType() === 'tabset' && !selectedId) {
        const tabset = node as import('@aptre/flex-layout').TabSetNode
        const selected = tabset.getSelectedNode()
        if (selected) {
          selectedId = selected.getId()
        }
      }
    })
  }

  return selectedId
}

// extractLocalStateFromModel extracts local state (active tabset, selections) from a model.
function extractLocalStateFromModel(model: Model): LayoutLocalState {
  const localState: LayoutLocalState = { tabSetSelections: {} }

  model.visitNodes((node) => {
    if (node.getType() === 'tabset') {
      const tabset = node as import('@aptre/flex-layout').TabSetNode
      if (tabset.isActive()) {
        localState.activeTabSetId = tabset.getId()
      }
      const selected = tabset.getSelectedNode()
      if (selected && localState.tabSetSelections) {
        localState.tabSetSelections[tabset.getId()] = selected.getId()
      }
      if (tabset.isMaximized()) {
        localState.maximizedTabSetId = tabset.getId()
      }
    }
  })

  return localState
}

// applyLocalStateToModel applies local state to a model.
export function applyLocalStateToModel(
  model: Model,
  localState: LayoutLocalState | undefined,
): void {
  if (!localState) return

  // Apply selected tabs
  if (localState.tabSetSelections) {
    for (const tabId of Object.values(localState.tabSetSelections)) {
      const node = model.getNodeById(tabId)
      if (node) {
        model.doAction(Actions.selectTab(tabId))
      }
    }
  }

  // Apply active tabset
  if (localState.activeTabSetId) {
    const node = model.getNodeById(localState.activeTabSetId)
    if (node) {
      model.doAction(Actions.setActiveTabset(localState.activeTabSetId))
    }
  }

  // Apply maximized state
  if (localState.maximizedTabSetId) {
    const node = model.getNodeById(localState.maximizedTabSetId)
    if (node) {
      model.doAction(Actions.maximizeToggle(localState.maximizedTabSetId))
    }
  }
}

// encodeGridLayout converts a Model to a URL-safe base64 string including local state.
export function encodeGridLayout(model: Model): string {
  const jsonModel = model.toJson()
  const tabDataMap: TabDataMap = {}
  const layoutModel = jsonModelToLayoutModel(jsonModel, tabDataMap)
  const localState = extractLocalStateFromModel(model)

  const snapshot: LayoutSnapshot = {
    model: layoutModel,
    localState,
  }

  const binary = LayoutSnapshot.toBinary(snapshot)

  // Convert to base64url encoding (URL-safe)
  const base64 = btoa(String.fromCharCode(...binary))
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// encodeGridLayoutStructure converts a Model to a URL-safe base64 string WITHOUT local state.
// Used for comparing if the structural layout has changed.
export function encodeGridLayoutStructure(model: Model): string {
  const jsonModel = model.toJson()
  const tabDataMap: TabDataMap = {}
  const layoutModel = jsonModelToLayoutModel(jsonModel, tabDataMap)

  // Encode only the structural model, no local state
  const snapshot: LayoutSnapshot = {
    model: layoutModel,
  }

  const binary = LayoutSnapshot.toBinary(snapshot)

  const base64 = btoa(String.fromCharCode(...binary))
  return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// DecodeResult contains the decoded model and local state.
export interface DecodeResult {
  model: IJsonModel
  localState?: LayoutLocalState
}

// decodeGridLayout converts a URL-safe base64 string back to an IJsonModel and local state.
export function decodeGridLayout(
  encoded: string,
  baseModel: IJsonModel,
): DecodeResult | null {
  if (!encoded) {
    return null
  }

  try {
    // Restore standard base64 from base64url
    let base64 = encoded.replace(/-/g, '+').replace(/_/g, '/')
    // Add padding if needed
    const padding = (4 - (base64.length % 4)) % 4
    base64 += '='.repeat(padding)

    const binary = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0))

    // Try to decode as LayoutSnapshot first (new format)
    try {
      const snapshot = LayoutSnapshot.fromBinary(binary)
      if (snapshot.model) {
        const tabDataMap: TabDataMap = {}
        const model = layoutModelToJsonModel(
          baseModel,
          tabDataMap,
          snapshot.model,
        )
        return { model, localState: snapshot.localState }
      }
    } catch {
      // Fall through to legacy format
    }

    // Fallback: try legacy LayoutModel format (no local state)
    const layoutModel = LayoutModel.fromBinary(binary)
    const tabDataMap: TabDataMap = {}
    const model = layoutModelToJsonModel(baseModel, tabDataMap, layoutModel)
    return { model }
  } catch {
    return null
  }
}

// getActiveTabsetId returns the ID of the active tabset in the model.
export function getActiveTabsetId(model: Model): string | null {
  let activeId: string | null = null

  model.visitNodes((node) => {
    if (node.getType() === 'tabset') {
      const tabset = node as import('@aptre/flex-layout').TabSetNode
      if (tabset.isActive()) {
        activeId = tabset.getId()
      }
    }
  })

  // Fallback to first tabset if none is active
  if (!activeId) {
    model.visitNodes((node) => {
      if (node.getType() === 'tabset' && !activeId) {
        activeId = node.getId()
      }
    })
  }

  return activeId
}
