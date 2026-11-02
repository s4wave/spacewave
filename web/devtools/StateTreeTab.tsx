import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'
import { useCallback, useMemo } from 'react'
import {
  LuChevronDown,
  LuChevronRight,
  LuDatabase,
} from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    paddingLeft: 'var(--debug-tree-padding-left)',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-pulldown-hover']} 40%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    overflow: 'auto',
  },
  rule2: { display: 'flex', flexDirection: 'column' },
  rule3: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.1s',
    '--tw-duration': '.1s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-pulldown-hover']} 40%, transparent)`,
      },
    },
  },
  rule4: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    marginLeft: 'auto',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule8: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule9: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule10: { visibility: 'hidden' },
  rule11: {
    display: 'flex',
    height: '100%',
    minWidth: 0,
    flex: 1,
    cursor: 'pointer',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    textAlign: 'left',
  },
  rule12: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
  rule13: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule14: {
    marginLeft: 'auto',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
})

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
    <div sx={stylexStyles.rule1}>
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
    <div sx={stylexStyles.rule2}>
      <button type="button" onClick={handleToggle} sx={[stylexStyles.rule3]}>
        <span sx={stylexStyles.rule4}>
          {isExpanded ? (
            <LuChevronDown sx={stylexStyles.rule5} />
          ) : (
            <LuChevronRight sx={stylexStyles.rule5} />
          )}
        </span>
        <span>{node.label}</span>
        <span sx={stylexStyles.rule6}>{node.count}</span>
      </button>
      {isExpanded && (
        <div sx={stylexStyles.rule2}>
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
    <div sx={stylexStyles.rule2}>
      <button
        type="button"
        onClick={handleToggle}
        sx={[stylexStyles2.rule1]}
        style={{ '--debug-tree-padding-left': `${level * 12 + 8}px` }}
      >
        <span sx={stylexStyles.rule4}>
          {isExpanded ? (
            <LuChevronDown sx={stylexStyles.rule7} />
          ) : (
            <LuChevronRight sx={stylexStyles.rule7} />
          )}
        </span>
        <span sx={stylexStyles.rule8}>{node.label}</span>
        <span sx={stylexStyles.rule6}>{node.count}</span>
      </button>
      {isExpanded && (
        <div sx={stylexStyles.rule2}>
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
  // Watch the entry value, selection and retained expansion state together.
  const devtools = useStateDevToolsContext()
  const selectedAtomId = useSelectedStateAtomId()
  const value = useStateInspectorValue(entry)
  const [expandedState, setExpandedState] = useTreeExpandedState()

  // Derive the row preview and expansion affordance from the current value.
  const isSelected = selectedAtomId === entry.id
  const isExpanded = expandedState[entry.id] ?? false
  const childEntries = getChildEntries(value)
  const hasChildren = childEntries.length > 0
  const preview = formatValuePreview(value)

  // Toggle this entry without changing other expanded rows.
  const handleToggle = useCallback(() => {
    setExpandedState((prev) => ({
      ...prev,
      [entry.id]: !prev[entry.id],
    }))
  }, [entry.id, setExpandedState])

  // Select the owning atom for the details panel.
  const handleSelect = useCallback(() => {
    devtools?.setSelectedAtomId(entry.id)
  }, [devtools, entry.id])

  // Render the row and only the descendants requested by its expansion state.
  return (
    <div sx={stylexStyles.rule2}>
      <div
        className={
          /* TODO(stylex): global or unknown class: debug-tree-indent */ cn(
            'debug-tree-indent flex h-6 items-center gap-1 text-xs transition-colors duration-100',
            isSelected
              ? 'bg-ui-selected text-foreground'
              : 'text-text-secondary hover:bg-pulldown-hover/50',
          )
        }
        style={{ '--debug-tree-padding-left': `${level * 12 + 8}px` }}
      >
        <button
          type="button"
          aria-label={isExpanded ? 'Collapse' : 'Expand'}
          disabled={!hasChildren}
          onClick={handleToggle}
          sx={[stylexStyles.rule9, !hasChildren && stylexStyles.rule10]}
        >
          {isExpanded ? (
            <LuChevronDown sx={stylexStyles.rule5} />
          ) : (
            <LuChevronRight sx={stylexStyles.rule5} />
          )}
        </button>

        <button type="button" onClick={handleSelect} sx={stylexStyles.rule11}>
          <LuDatabase sx={stylexStyles.rule12} />
          <span sx={stylexStyles.rule13}>{label}</span>
          <span sx={stylexStyles.rule14}>{preview}</span>
        </button>
      </div>

      {isExpanded && hasChildren && (
        <div sx={stylexStyles.rule2}>
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
  // Share selection and expansion with the containing inspector.
  const devtools = useStateDevToolsContext()
  const [expandedState, setExpandedState] = useTreeExpandedState()

  // Give each nested value its own expansion key within the atom.
  const pathKey = `${atomId}:${path.join('.')}`
  const isExpanded = expandedState[pathKey] ?? false
  const childEntries = getChildEntries(value)
  const isExpandable = childEntries.length > 0

  // Toggle this value path while preserving sibling expansion.
  const handleToggle = useCallback(() => {
    setExpandedState((prev) => ({
      ...prev,
      [pathKey]: !prev[pathKey],
    }))
  }, [pathKey, setExpandedState])

  // Select both the owning atom and this nested path.
  const handleSelect = useCallback(() => {
    devtools?.setSelectedAtomId(atomId)
    devtools?.setSelectedPath(path)
  }, [atomId, devtools, path])

  // Summarize the current value without rendering its descendants.
  const displayValue = formatValuePreview(value)
  const valueColor = getValueColor(value)

  // Render the row and only the descendants requested by its expansion state.
  return (
    <div sx={stylexStyles.rule2}>
      <div
        className={
          /* TODO(stylex): global or unknown class: debug-tree-indent */ cn(
            'debug-tree-indent flex h-5 items-center gap-1 text-xs',
            'text-text-secondary hover:bg-pulldown-hover/50 transition-colors duration-100',
          )
        }
        style={{ '--debug-tree-padding-left': `${level * 12 + 8}px` }}
      >
        <button
          type="button"
          aria-label={isExpanded ? 'Collapse' : 'Expand'}
          disabled={!isExpandable}
          onClick={handleToggle}
          sx={[stylexStyles.rule9, !isExpandable && stylexStyles.rule10]}
        >
          {isExpanded ? (
            <LuChevronDown sx={stylexStyles.rule7} />
          ) : (
            <LuChevronRight sx={stylexStyles.rule7} />
          )}
        </button>

        <button type="button" onClick={handleSelect} sx={stylexStyles.rule11}>
          <span sx={stylexStyles.rule15}>{nodeKey}:</span>
          <span
            className={
              /* TODO(stylex): computed class helper */ cn(
                'truncate font-mono text-xs',
                valueColor,
              )
            }
          >
            {displayValue}
          </span>
        </button>
      </div>

      {isExpanded && isExpandable && (
        <div sx={stylexStyles.rule2}>
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
  // Index group prefixes while keeping each scope's root children in display order.
  const groupMap = new Map<string, GroupNode>()
  const rootNodes: ChildNode[] = []

  // Attach each entry beneath the groups named by its label.
  for (const entry of entries) {
    // Build the entry's group path before appending its leaf label.
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

  // Count descendant entries before sorting the scope for display.
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
  return nodes
    .toSorted((a, b) => {
      if (a.kind === 'entry' && b.kind !== 'entry') return 1
      if (a.kind !== 'entry' && b.kind === 'entry') return -1
      return a.label.localeCompare(b.label)
    })
    .map((node) =>
      node.kind === 'entry'
        ? node
        : {
            ...node,
            children: sortNodes(node.children),
          },
    )
}

function useTreeExpandedState() {
  const environment = useAppEnvironment()
  return useStateAtom<Record<string, boolean>>(
    {
      namespace: ['devtools', 'state', 'tree'],
      stateAtom: environment.id ? environment.rootAtom : stateDevToolsStateAtom,
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
  // Keep absent values and scalar text distinguishable in the inspector.
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
  // Use muted tones for absence and semantic tones for scalar values.
  if (value === null || value === undefined) return 'text-foreground-alt'
  if (typeof value === 'string') return 'text-console-output'
  if (typeof value === 'number') return 'text-console-info'
  if (typeof value === 'boolean') return 'text-brand'
  return 'text-text-secondary'
}
