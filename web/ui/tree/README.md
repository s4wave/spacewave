# Tree Component

A hierarchical tree component with keyboard navigation, multi-selection, and state persistence.

## Usage

```tsx
import { Tree, TreeNode } from '@s4wave/web/ui/tree'

const nodes: TreeNode[] = [
  {
    id: 'folder1',
    name: 'Documents',
    children: [
      { id: 'file1', name: 'notes.txt' },
      { id: 'file2', name: 'todo.md' },
    ],
  },
  { id: 'folder2', name: 'Images' },
]

function MyTree() {
  return (
    <Tree
      nodes={nodes}
      defaultExpandedIds={new Set(['folder1'])}
      onRowDefaultAction={(nodes) => console.log('Opened:', nodes)}
    />
  )
}
```

## Props

| Prop | Type | Description |
|------|------|-------------|
| `nodes` | `TreeNode<T>[]` | Tree data structure |
| `placeholder` | `ReactNode` | Shown when nodes is empty |
| `className` | `string` | Additional CSS classes |
| `onRowDefaultAction` | `(nodes: TreeNode<T>[]) => void` | Called on double-click or Enter |
| `defaultExpandedIds` | `Set<string>` | Initially expanded node IDs |
| `defaultSelectedIds` | `Set<string>` | Initially selected node IDs |
| `namespace` | `StateNamespace` | For state persistence |
| `stateKey` | `string` | Key for persisted state (default: 'tree') |
| `defaultState` | `Partial<TreeState>` | Override default state values |

## TreeNode Interface

```tsx
interface TreeNode<T = void> {
  id: string
  name: string
  data?: T
  icon?: ReactNode
  icons?: { icon: ReactNode; tooltip?: string; onClick?: () => void }[]
  children?: TreeNode<T>[]
  onDragStart?: (e: DragEvent, node: TreeNode<T>, state: TreeState) => void
}
```

## Keyboard Navigation

| Key | Action |
|-----|--------|
| `j` / `ArrowDown` | Move to next visible node |
| `k` / `ArrowUp` | Move to previous visible node |
| `l` / `ArrowRight` | Expand node or move to first child |
| `h` / `ArrowLeft` | Collapse node or move to parent |
| `Enter` | Trigger default action on selected nodes |
| `Space` | Toggle expand on parent nodes |
| `Home` | Jump to first node |
| `End` | Jump to last visible node |
| `Shift+Arrow` | Range selection |
| `Ctrl/Cmd+Click` | Toggle selection |

## State Persistence

Pass a `namespace` prop to persist expand/selection state:

```tsx
import { useStateNamespace } from '@s4wave/web/state/persist.js'

function PersistentTree() {
  const namespace = useStateNamespace(['my-tree'])
  
  return (
    <Tree
      nodes={nodes}
      namespace={namespace}
      stateKey="tree"
    />
  )
}
```

## Accessing State from Custom Components

Use the exported contexts to access tree state:

```tsx
import { useContext } from 'react'
import { TreeStateContext, TreeDispatchContext } from '@s4wave/web/ui/tree'

function CustomTreeWidget() {
  const state = useContext(TreeStateContext)
  const dispatch = useContext(TreeDispatchContext)
  
  const selectedCount = state?.selectedIds.size ?? 0
  
  return <div>{selectedCount} selected</div>
}
```

## Exports

```tsx
// Components
export { Tree } from './Tree.js'
export { TreeRow } from './TreeRow.js'

// Types
export type { TreeProps } from './Tree.js'
export type { TreeNode, TreeNodeOnDragStart } from './TreeNode.js'
export type { TreeState, TreeAction, SelectNodeAction, TreeDispatch } from './TreeState.js'

// Contexts
export { TreeStateContext, TreeDispatchContext } from './TreeState.js'

// Utilities
export { treeReducer, findNodeById, findParentNode, getVisibleNodes } from './TreeState.js'
```
