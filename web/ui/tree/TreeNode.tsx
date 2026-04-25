import React, { DragEvent } from 'react'
import { TreeState } from './TreeState.js'

export interface TreeNode<T = void> {
  id: string
  name: string
  icon?: React.ReactNode
  children?: TreeNode<T>[]
  data?: T
  draggable?: boolean
  onDragStart?: TreeNodeOnDragStart<T>
  icons?: {
    icon: React.ReactNode
    onClick?: (e: React.MouseEvent) => void
    tooltip?: string
  }[]
}

export type TreeNodeOnDragStart<T> = (
  event: DragEvent<HTMLElement>,
  node: TreeNode<T>,
  state: TreeState,
) => void
