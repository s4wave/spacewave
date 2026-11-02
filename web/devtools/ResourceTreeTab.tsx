import { useCallback, useEffect, useMemo, useState } from 'react'

import {
  Tree,
  useTreeState,
  type TreeNode,
  type TreeUpdate,
} from '@s4wave/web/ui/tree/index.js'
import { useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  useResourceDevToolsContext,
  useTrackedResources,
  getResourceLabel,
  type TrackedResource,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    overflow: 'hidden',
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-popover-border']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-deep']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule4: { flex: 1 },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 2)`,
    flexShrink: 0,
    borderRadius: '3.40282e38px',
  },
  rule6: { backgroundColor: themeColors['--color-success'] },
  rule7: { backgroundColor: themeColors['--color-foreground-alt'] },
  rule8: { backgroundColor: themeColors['--color-warning'] },
  rule9: { backgroundColor: themeColors['--color-error'] },
})

// ResourceTreeTab displays resources in a hierarchical tree view.
export function ResourceTreeTab() {
  const devtools = useResourceDevToolsContext()
  const resources = useTrackedResources()
  const namespace = useStateNamespace(['devtools', 'resources', 'tree'])

  // Force refresh when there are loading resources (to update duration display)
  const [tick, setTick] = useState(0)
  const hasLoadingResources = useMemo(
    () => Array.from(resources.values()).some((r) => r.state === 'loading'),
    [resources],
  )
  useEffect(() => {
    if (!hasLoadingResources) return
    const interval = setInterval(() => setTick((t) => t + 1), 1000)
    return () => clearInterval(interval)
  }, [hasLoadingResources])

  const nodes = useMemo(() => {
    // Include tick to force rebuild when timer fires
    void tick
    return buildTreeNodes(resources)
  }, [resources, tick])

  // Each Tree update shows the first selected resource in the details panel.
  // The state atom setter applies the update before it returns.
  const [treeState, setTreeState] = useTreeState(namespace, 'state')
  const updateTree = useCallback<TreeUpdate>(
    (update) => {
      let selectedIds = treeState.selectedIds
      setTreeState((prev) => {
        const next = update(prev)
        selectedIds = next.selectedIds
        return next
      })
      devtools?.setSelectedId(selectedIds.values().next().value ?? null)
    },
    [devtools, treeState.selectedIds, setTreeState],
  )

  const resourceCount = resources.size

  return (
    <div sx={stylexStyles.rule1}>
      {/* Section header */}
      <div sx={[stylexStyles.rule2]}>
        <span
          className={
            /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs font-medium'
          }
        >
          Resources
        </span>
        <span sx={stylexStyles.rule3}>{resourceCount}</span>
      </div>

      {/* Tree content */}
      <Tree
        nodes={nodes}
        state={[treeState, updateTree]}
        placeholder="No resources tracked"
        sx={stylexStyles.rule4}
      />
    </div>
  )
}

// buildTreeNodes converts tracked resources to tree nodes.
// Resources with multiple parents are only shown under their first parent to avoid duplicates.
function buildTreeNodes(
  resources: Map<string, TrackedResource>,
): TreeNode<TrackedResource>[] {
  // Track which resources have been added to the tree to prevent duplicates
  // (a resource with multiple parents would otherwise appear under each parent)
  const addedIds = new Set<string>()
  const childrenByParentId = new Map<string, TrackedResource[]>()
  for (const resource of resources.values()) {
    for (const parentId of resource.parentIds) {
      const children = childrenByParentId.get(parentId)
      if (children) {
        children.push(resource)
        continue
      }
      childrenByParentId.set(parentId, [resource])
    }
  }

  function buildNode(resource: TrackedResource): TreeNode<TrackedResource> {
    addedIds.add(resource.id)

    // Find children that list this resource as a parent and haven't been added yet
    // We must check addedIds inside the loop (not just filter) because buildNode
    // modifies addedIds, and we need to see updates from sibling subtrees
    const children: TreeNode<TrackedResource>[] = []
    for (const r of childrenByParentId.get(resource.id) ?? []) {
      if (!addedIds.has(r.id)) {
        children.push(buildNode(r))
      }
    }

    return {
      id: resource.id,
      name: getResourceLabel(resource),
      icon: (
        <StateIndicator state={resource.state} released={resource.released} />
      ),
      data: resource,
      children: children.length > 0 ? children : undefined,
    }
  }

  // Find root resources (no parents or all parents missing from map)
  const roots = Array.from(resources.values()).filter(
    (r) =>
      r.parentIds.length === 0 ||
      r.parentIds.every((pid) => !resources.has(pid)),
  )

  // Build nodes for each root, collecting results
  const result: TreeNode<TrackedResource>[] = []
  for (const root of roots) {
    // Skip if this root was already added as a child of a previous root
    if (addedIds.has(root.id)) continue
    result.push(buildNode(root))
  }

  return result
}

interface StateIndicatorProps {
  state: 'loading' | 'ready' | 'error'
  released: boolean
}

// StateIndicator renders a colored dot for resource state.
function StateIndicator({ state, released }: StateIndicatorProps) {
  return (
    <span
      sx={[
        stylexStyles.rule5,
        !!(state === 'ready' && !released) && stylexStyles.rule6,
        !!(state === 'ready' && released) && stylexStyles.rule7,
        state === 'loading' && stylexStyles.rule8,
        state === 'error' && stylexStyles.rule9,
      ]}
      title={released ? `${state} (released)` : state}
    />
  )
}
