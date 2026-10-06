# Tree Component

A hierarchical tree component with keyboard navigation, multi-selection, and state persistence.

## Usage

```tsx
import { Tree, TreeNode, useTreeState } from '@s4wave/web/ui/tree'

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
  const state = useTreeState(null, 'tree', {
    expandedIds: new Set(['folder1']),
  })
  return (
    <Tree
      nodes={nodes}
      state={state}
      onRowDefaultAction={(nodes) => console.log('Opened:', nodes)}
    />
  )
}
```

## Props

| Prop | Type | Description |
|------|------|-------------|
| `nodes` | `TreeNode<T>[]` | Tree data structure |
| `state` | `TreeStateHandle` | The tree state and its updater, from `useTreeState` |
| `placeholder` | `ReactNode` | Shown when nodes is empty |
| `className` | `string` | Additional CSS classes |
| `onRowDefaultAction` | `(nodes: TreeNode<T>[]) => void` | Called on double-click or Enter |

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

## State

The component that renders the Tree owns its state. `useTreeState(namespace,
key, initial?)` returns the `[state, update]` handle the Tree renders. The owner
reads the expanded and selected IDs directly and changes them with
`update((prev) => next)`. With a `namespace` the state persists under `key`;
with `null` it lives in memory.

```tsx
import { useStateNamespace } from '@s4wave/web/state/persist.js'

function PersistentTree() {
  const namespace = useStateNamespace(['my-tree'])
  const state = useTreeState(namespace, 'tree')
  const [{ selectedIds }] = state

  return (
    <>
      <Tree nodes={nodes} state={state} />
      <div>{selectedIds.size} selected</div>
    </>
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
export type { TreeState, TreeStateHandle, TreeUpdate, TreeAction, SelectNodeAction, TreeDispatch } from './TreeState.js'

// Contexts
export { TreeStateContext, TreeDispatchContext } from './TreeState.js'

// Utilities
export { useTreeState, treeReducer, findNodeById, findParentNode, getVisibleNodes } from './TreeState.js'
```
