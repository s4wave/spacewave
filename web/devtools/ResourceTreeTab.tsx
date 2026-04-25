import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { Tree, type TreeNode } from '@s4wave/web/ui/tree/index.js'
import { useStateNamespace } from '@s4wave/web/state/persist.js'
import {
  useResourceDevToolsContext,
  useTrackedResources,
  getResourceLabel,
  type TrackedResource,
} from '@aptre/bldr-sdk/hooks/ResourceDevToolsContext.js'

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

  const devtoolsRef = useRef(devtools)
  useEffect(() => {
    devtoolsRef.current = devtools
  }, [devtools])

  const handleSelectionChange = useCallback((selectedIds: Set<string>) => {
    const dt = devtoolsRef.current
    if (!dt) return
    const firstId = selectedIds.values().next().value
    dt.setSelectedId(firstId ?? null)
  }, [])

  const resourceCount = resources.size

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Section header */}
      <div
        className={cn(
          'border-popover-border/50 flex h-6 shrink-0 items-center justify-between border-b px-2',
          'bg-background-deep/30',
        )}
      >
        <span className="text-text-secondary text-xs font-medium">
          Resources
        </span>
        <span className="text-foreground-alt font-mono text-xs">
          {resourceCount}
        </span>
      </div>

      {/* Tree content */}
      <Tree
        nodes={nodes}
        namespace={namespace}
        stateKey="state"
        placeholder="No resources tracked"
        onSelectionChange={handleSelectionChange}
        className="flex-1"
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

  function buildNode(resource: TrackedResource): TreeNode<TrackedResource> {
    addedIds.add(resource.id)

    // Find children that list this resource as a parent and haven't been added yet
    // We must check addedIds inside the loop (not just filter) because buildNode
    // modifies addedIds, and we need to see updates from sibling subtrees
    const children: TreeNode<TrackedResource>[] = []
    for (const r of resources.values()) {
      if (r.parentIds.includes(resource.id) && !addedIds.has(r.id)) {
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
      className={cn(
        'h-2 w-2 shrink-0 rounded-full',
        state === 'ready' && !released && 'bg-success',
        state === 'ready' && released && 'bg-foreground-alt',
        state === 'loading' && 'bg-warning',
        state === 'error' && 'bg-error',
      )}
      title={released ? `${state} (released)` : state}
    />
  )
}
