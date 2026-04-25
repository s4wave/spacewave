import type { ReactNode } from 'react'

// NodeType matches proto NodeType enum.
export type NodeType = 'text' | 'shape' | 'world_object' | 'drawing'

// EdgeStyle matches proto EdgeStyle enum.
export type EdgeStyle = 'bezier' | 'straight'

// CanvasNodeData represents a node on the canvas.
export interface CanvasNodeData {
  id: string
  x: number
  y: number
  width: number
  height: number
  zIndex: number
  type: NodeType
  textContent?: string
  shapeData?: Uint8Array
  objectKey?: string
  pinned?: boolean
  viewPath?: string
}

// CanvasEdgeData represents a user-drawn visual-only edge.
export interface CanvasEdgeData {
  id: string
  sourceNodeId: string
  targetNodeId: string
  label?: string
  style: EdgeStyle
}

// HiddenGraphLinkData is a canvas-scoped hidden world graph link identity.
export interface HiddenGraphLinkData {
  subject: string
  predicate: string
  object: string
  label?: string
}

// EphemeralEdge is a world graph edge shown when a node is selected.
export interface EphemeralEdge {
  renderKey: string
  subject: string
  predicate: string
  object: string
  label?: string
  sourceNodeId: string
  sourceObjectKey: string
  sourceGroupKey: string
  sourceGroupIndex: number
  sourceGroupOffset: number
  outgoingTruncated: boolean
  incomingTruncated: boolean
  hiddenCount: number
  direction: 'out' | 'in'
  linkedObjectKey: string
  linkedObjectLabel: string
  linkedObjectType?: string
  linkedObjectTypeLabel?: string
  hideable: boolean
  userRemovable: boolean
  protected: boolean
  ownerManaged: boolean
  // If the linked object is already on the canvas, its node ID.
  targetNodeId?: string
  // If not on canvas, position for the stub.
  stubX?: number
  stubY?: number
}

// Viewport represents the canvas pan/zoom state.
export interface Viewport {
  x: number
  y: number
  scale: number
}

// CanvasStateData is the full state of the canvas.
export interface CanvasStateData {
  nodes: Map<string, CanvasNodeData>
  edges: CanvasEdgeData[]
  hiddenGraphLinks: HiddenGraphLinkData[]
}

// CanvasTool is the active tool in the toolbar.
export type CanvasTool = 'select' | 'draw' | 'text' | 'object'

// CanvasAction is a named action in the action map.
export type CanvasAction =
  | 'delete'
  | 'copy'
  | 'paste'
  | 'undo'
  | 'redo'
  | 'select-all'
  | 'deselect'
  | 'zoom-in'
  | 'zoom-out'
  | 'zoom-reset'
  | 'fit-view'
  | 'bring-to-front'
  | 'send-to-back'

// SIZE_THRESHOLD_WIDTH is the minimum unscaled node width for full content.
export const SIZE_THRESHOLD_WIDTH = 200

// SIZE_THRESHOLD_HEIGHT is the minimum unscaled node height for full content.
export const SIZE_THRESHOLD_HEIGHT = 150

// VIEWPORT_MARGIN is the margin in pixels around the viewport for virtualization.
export const VIEWPORT_MARGIN = 500

// UNMOUNT_DEBOUNCE_MS is the delay before unmounting hidden nodes.
export const UNMOUNT_DEBOUNCE_MS = 2000

// SEMANTIC_ZOOM_OUTLINE is the zoom threshold below which nodes show outlines only.
export const SEMANTIC_ZOOM_OUTLINE = 0.3

// SEMANTIC_ZOOM_MAX is the zoom threshold above which internal scaling is capped.
export const SEMANTIC_ZOOM_MAX = 2

// MIN_SCALE is the minimum zoom level.
export const MIN_SCALE = 0.1

// MAX_SCALE is the maximum zoom level.
export const MAX_SCALE = 5

// ZOOM_STEPS is the set of discrete zoom levels for toolbar zoom in/out.
export const ZOOM_STEPS = [0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 5]

// CanvasCallbacks are callbacks the consumer provides to handle canvas events.
export interface CanvasCallbacks {
  // onNodesChange is called with only the changed/added nodes (not the full map).
  onNodesChange?: (nodes: Map<string, CanvasNodeData>) => void
  // onNodesRemove is called with the IDs of nodes to remove.
  onNodesRemove?: (nodeIds: string[]) => void
  onEdgesChange?: (edges: CanvasEdgeData[]) => void
  onNodeSelect?: (nodeIds: Set<string>) => void
  onPinObject?: (objectKey: string, x: number, y: number) => void
  onFocusObject?: (objectKey: string, nodeId: string) => void
  onHideGraphLink?: (link: EphemeralEdge) => void
  onDeleteGraphLink?: (link: EphemeralEdge) => void
  // renderNodeContent is called for each visible node to render custom content.
  // This is how the ObjectType viewer injects ObjectViewers.
  renderNodeContent?: (node: CanvasNodeData) => ReactNode
}
