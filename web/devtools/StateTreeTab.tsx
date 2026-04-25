import { useCallback, useMemo } from 'react'
import { LuChevronDown, LuChevronRight, LuDatabase } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { useStateAtom } from '@s4wave/web/state/persist.js'
import {
  useSelectedStateAtomId,
  useStateDevToolsContext,
  stateDevToolsStateAtom,
} from './StateDevToolsContext.js'
import {
  useStateInspectorEntries,
  useStateInspectorValue,
  type StateInspectorEntry,
  type StateInspectorScope,
} from './useStateInspectorEntries.js'

type ScopeNode = {
  count: number
  id: string
  kind: 'scope'
  label: string
  scope: StateInspectorScope
  children: ChildNode[]
}

type GroupNode = {
  count: number
  id: string
  kind: 'group'
  label: string
  scope: StateInspectorScope
  children: ChildNode[]
}

type EntryNode = {
  id: string
  kind: 'entry'
  label: string
  entry: StateInspectorEntry
}

type ChildNode = GroupNode | EntryNode
type TreeNode = ScopeNode | ChildNode

const SCOPE_LABELS: Record<StateInspectorScope, string> = {
  local: 'Local State',
  persistent: 'Persistent State',
  root: 'Root State',
  session: 'Session State',
}

// StateTreeTab displays state atoms and their contents in a grouped tree view.
export function StateTreeTab() {
  const entries = useStateInspectorEntries()
  const tree = useMemo(() => buildTree(entries), [entries])

  return (
    <div className="flex flex-1 flex-col overflow-auto">
      {tree.map((node) => (
        <StateScopeNode key={node.id} node={node} />
      ))}
    </div>
  )
}

function StateScopeNode({ node }: { node: ScopeNode }) {
  const [expandedState, setExpandedState] = useTreeExpandedState()
  const isExpanded = expandedState[node.id] ?? true

  const handleToggle = useCallback(() => {
    setExpandedState((prev) => ({
      ...prev,
      [node.id]: !prev[node.id],
    }))
  }, [node.id, setExpandedState])

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={handleToggle}
        className={cn(
          'border-foreground/6 text-foreground flex h-7 items-center gap-1 border-b px-2 text-xs font-medium',
          'hover:bg-pulldown-hover/40 transition-colors duration-100',
        )}
      >
        <span className="flex h-4 w-4 shrink-0 items-center justify-center">
          {isExpanded ?
            <LuChevronDown className="h-3 w-3" />
          : <LuChevronRight className="h-3 w-3" />}
        </span>
        <span>{node.label}</span>
        <span className="text-foreground-alt ml-auto font-mono text-xs">
          {node.count}
        </span>
      </button>
      {isExpanded && (
        <div className="flex flex-col">
          {node.children.map((child) => (
            <StateTreeNode key={child.id} node={child} level={1} />
          ))}
        </div>
      )}
    </div>
  )
}

function StateTreeNode({ node, level }: { node: ChildNode; level: number }) {
  if (node.kind === 'entry') {
    return (
      <StateEntryNode entry={node.entry} label={node.label} level={level} />
    )
  }
  return <StateGroupNode node={node} level={level} />
}

function StateGroupNode({ node, level }: { node: GroupNode; level: number }) {
  const [expandedState, setExpandedState] = useTreeExpandedState()
  const isExpanded = expandedState[node.id] ?? true

  const handleToggle = useCallback(() => {
    setExpandedState((prev) => ({
      ...prev,
      [node.id]: !prev[node.id],
    }))
  }, [node.id, setExpandedState])

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={handleToggle}
        className={cn(
          'text-foreground-alt hover:bg-pulldown-hover/40 flex h-6 items-center gap-1 text-xs transition-colors duration-100',
        )}
        style={{ paddingLeft: `${level * 12 + 8}px` }}
      >
        <span className="flex h-4 w-4 shrink-0 items-center justify-center">
          {isExpanded ?
            <LuChevronDown className="h-2.5 w-2.5" />
          : <LuChevronRight className="h-2.5 w-2.5" />}
        </span>
        <span className="truncate">{node.label}</span>
        <span className="text-foreground-alt ml-auto font-mono text-xs">
          {node.count}
        </span>
      </button>
      {isExpanded && (
        <div className="flex flex-col">
          {node.children.map((child) => (
            <StateTreeNode key={child.id} node={child} level={level + 1} />
          ))}
        </div>
      )}
    </div>
  )
}

function StateEntryNode({
  entry,
  label,
  level,
}: {
  entry: StateInspectorEntry
  label: string
  level: number
}) {
  const devtools = useStateDevToolsContext()
  const selectedAtomId = useSelectedStateAtomId()
  const value = useStateInspectorValue(entry)
  const [expandedState, setExpandedState] = useTreeExpandedState()

  const isSelected = selectedAtomId === entry.id
  const isExpanded = expandedState[entry.id] ?? false
  const childEntries = getChildEntries(value)
  const hasChildren = childEntries.length > 0
  const preview = formatValuePreview(value)

  const handleToggle = useCallback(() => {
    setExpandedState((prev) => ({
      ...prev,
      [entry.id]: !prev[entry.id],
    }))
  }, [entry.id, setExpandedState])

  const handleSelect = useCallback(() => {
    devtools?.setSelectedAtomId(entry.id)
  }, [devtools, entry.id])

  return (
    <div className="flex flex-col">
      <div
        role="button"
        tabIndex={0}
        onClick={handleSelect}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            handleSelect()
          }
        }}
        className={cn(
          'flex h-6 cursor-pointer items-center gap-1 text-xs transition-colors duration-100',
          isSelected ?
            'bg-ui-selected text-foreground'
          : 'text-text-secondary hover:bg-pulldown-hover/50',
        )}
        style={{ paddingLeft: `${level * 12 + 8}px` }}
      >
        <span
          role="button"
          tabIndex={hasChildren ? 0 : -1}
          onClick={(e) => {
            e.stopPropagation()
            if (hasChildren) handleToggle()
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              e.stopPropagation()
              if (hasChildren) handleToggle()
            }
          }}
          className={cn(
            'flex h-4 w-4 shrink-0 items-center justify-center',
            !hasChildren && 'invisible',
          )}
        >
          {isExpanded ?
            <LuChevronDown className="h-3 w-3" />
          : <LuChevronRight className="h-3 w-3" />}
        </span>

        <LuDatabase className="text-brand h-3 w-3 shrink-0" />
        <span className="truncate text-xs font-medium">{label}</span>
        <span className="text-foreground-alt ml-auto truncate font-mono text-xs">
          {preview}
        </span>
      </div>

      {isExpanded && hasChildren && (
        <div className="flex flex-col">
          {childEntries.map(([key, childValue]) => (
            <StateTreeNodeInner
              key={key}
              atomId={entry.id}
              level={level + 1}
              nodeKey={key}
              path={[key]}
              value={childValue}
            />
          ))}
        </div>
      )}
    </div>
  )
}

interface StateTreeNodeInnerProps {
  atomId: string
  level: number
  nodeKey: string
  path: string[]
  value: unknown
}

function StateTreeNodeInner({
  atomId,
  level,
  nodeKey,
  path,
  value,
}: StateTreeNodeInnerProps) {
  const devtools = useStateDevToolsContext()
  const [expandedState, setExpandedState] = useTreeExpandedState()

  const pathKey = `${atomId}:${path.join('.')}`
  const isExpanded = expandedState[pathKey] ?? false
  const childEntries = getChildEntries(value)
  const isExpandable = childEntries.length > 0

  const handleToggle = useCallback(() => {
    setExpandedState((prev) => ({
      ...prev,
      [pathKey]: !prev[pathKey],
    }))
  }, [pathKey, setExpandedState])

  const handleSelect = useCallback(() => {
    devtools?.setSelectedAtomId(atomId)
    devtools?.setSelectedPath(path)
  }, [atomId, devtools, path])

  const displayValue = formatValuePreview(value)
  const valueColor = getValueColor(value)

  return (
    <div className="flex flex-col">
      <div
        role="button"
        tabIndex={0}
        onClick={handleSelect}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            handleSelect()
          }
        }}
        className={cn(
          'flex h-5 cursor-pointer items-center gap-1 text-xs',
          'text-text-secondary hover:bg-pulldown-hover/50 transition-colors duration-100',
        )}
        style={{ paddingLeft: `${level * 12 + 8}px` }}
      >
        <span
          role="button"
          tabIndex={isExpandable ? 0 : -1}
          onClick={(e) => {
            e.stopPropagation()
            if (isExpandable) handleToggle()
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              e.stopPropagation()
              if (isExpandable) handleToggle()
            }
          }}
          className={cn(
            'flex h-4 w-4 shrink-0 items-center justify-center',
            !isExpandable && 'invisible',
          )}
        >
          {isExpanded ?
            <LuChevronDown className="h-2.5 w-2.5" />
          : <LuChevronRight className="h-2.5 w-2.5" />}
        </span>

        <span className="text-foreground font-mono text-xs">{nodeKey}:</span>
        <span className={cn('truncate font-mono text-xs', valueColor)}>
          {displayValue}
        </span>
      </div>

      {isExpanded && isExpandable && (
        <div className="flex flex-col">
          {childEntries.map(([childKey, childValue]) => (
            <StateTreeNodeInner
              key={childKey}
              atomId={atomId}
              level={level + 1}
              nodeKey={childKey}
              path={[...path, childKey]}
              value={childValue}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function buildTree(entries: StateInspectorEntry[]): ScopeNode[] {
  return (Object.keys(SCOPE_LABELS) as StateInspectorScope[]).map((scope) => {
    const children = buildScopeChildren(
      entries.filter((entry) => entry.scope === scope),
      scope,
    )
    return {
      count: countLeafNodes(children),
      id: `scope:${scope}`,
      kind: 'scope',
      label: SCOPE_LABELS[scope],
      scope,
      children,
    }
  })
}

function buildScopeChildren(
  entries: StateInspectorEntry[],
  scope: StateInspectorScope,
): ChildNode[] {
  const groupMap = new Map<string, GroupNode>()
  const rootNodes: ChildNode[] = []

  for (const entry of entries) {
    const segments = entry.label.split('/').filter(Boolean)
    const leafLabel = segments[segments.length - 1] ?? entry.label
    const groupSegments = segments.slice(0, -1)
    let parentChildren = rootNodes
    let pathSoFar: string[] = []

    for (const segment of groupSegments) {
      pathSoFar = [...pathSoFar, segment]
      const groupId = `group:${scope}:${pathSoFar.join('/')}`
      let groupNode = groupMap.get(groupId)
      if (!groupNode) {
        groupNode = {
          count: 0,
          id: groupId,
          kind: 'group',
          label: segment,
          scope,
          children: [],
        }
        groupMap.set(groupId, groupNode)
        parentChildren.push(groupNode)
      }
      parentChildren = groupNode.children
    }

    parentChildren.push({
      id: entry.id,
      kind: 'entry',
      label: leafLabel,
      entry,
    })
  }

  finalizeCounts(rootNodes)
  return sortNodes(rootNodes)
}

function finalizeCounts(nodes: ChildNode[]): number {
  let total = 0
  for (const node of nodes) {
    if (node.kind === 'entry') {
      total += 1
      continue
    }
    node.count = finalizeCounts(node.children)
    total += node.count
  }
  return total
}

function countLeafNodes(nodes: ChildNode[]): number {
  return nodes.reduce((sum, node) => {
    if (node.kind === 'entry') return sum + 1
    return sum + countLeafNodes(node.children)
  }, 0)
}

function sortNodes(nodes: ChildNode[]): ChildNode[] {
  return [...nodes]
    .sort((a, b) => {
      if (a.kind === 'entry' && b.kind !== 'entry') return 1
      if (a.kind !== 'entry' && b.kind === 'entry') return -1
      return a.label.localeCompare(b.label)
    })
    .map((node) =>
      node.kind === 'entry' ?
        node
      : {
          ...node,
          children: sortNodes(node.children),
        },
    )
}

function useTreeExpandedState() {
  return useStateAtom<Record<string, boolean>>(
    {
      namespace: ['devtools', 'state', 'tree'],
      stateAtom: stateDevToolsStateAtom,
    },
    'expanded',
    {},
  )
}

function getChildEntries(value: unknown): Array<[string, unknown]> {
  if (Array.isArray(value)) {
    return value.map((childValue, index) => [String(index), childValue])
  }
  if (value !== null && typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>)
  }
  return []
}

function formatValuePreview(value: unknown): string {
  if (value === null) return 'null'
  if (value === undefined) return 'undefined'
  if (typeof value === 'string') return `"${value}"`
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value)
  }
  if (Array.isArray(value)) return `Array(${value.length})`
  if (typeof value === 'object') return `{${Object.keys(value).length}}`
  return Object.prototype.toString.call(value)
}

function getValueColor(value: unknown): string {
  if (value === null || value === undefined) return 'text-foreground-alt'
  if (typeof value === 'string') return 'text-console-output'
  if (typeof value === 'number') return 'text-console-info'
  if (typeof value === 'boolean') return 'text-brand'
  return 'text-text-secondary'
}
