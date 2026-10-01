import React, { DragEvent } from 'react'
import { TreeState } from './TreeState.js'

export interface TreeNode<T = void> {
  id: string
  name: string
  detail?: string
  icon?: React.ReactNode
  children?: TreeNode<T>[]
  // hasChildren marks a node as expandable before its children are loaded.
  hasChildren?: boolean
  data?: T
  draggable?: boolean
  onDragStart?: TreeNodeOnDragStart<T>
  icons?: {
    icon: React.ReactNode
    onClick?: (e: React.MouseEvent) => void
    tooltip?: string
  }[]
}

// isExpandable returns whether a node has loaded or not-yet-loaded children.
export function isExpandable<T>(node: TreeNode<T>): boolean {
  return !!node.children?.length || !!node.hasChildren
}

export type TreeNodeOnDragStart<T> = (
  event: DragEvent<HTMLElement>,
  node: TreeNode<T>,
  state: TreeState,
) => void
