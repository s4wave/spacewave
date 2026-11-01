import { useMemo, useState, type ReactNode } from 'react'
import { LuChevronRight, LuCornerLeftUp } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import {
  directiveMatches,
  relevantDirectives,
  type DirectiveOriginGroup,
  type DirectiveEdge,
  type DirectiveNode,
  type DirectiveTree,
} from './directiveTree.js'
import { formatCount } from './format.js'

// DirectiveTreeView draws the session bus's directives under their origins.
// A directive with several origins appears under each one.
// A directive that sits above itself is drawn again as a back link and not expanded.
export function DirectiveTreeView({
  tree,
  query,
  configKey,
}: {
  tree: DirectiveTree
  query: string
  configKey: string
}) {
  const relevant = useMemo(() => relevantDirectives(tree, query), [tree, query])
  const keep = (edge: DirectiveEdge) => !relevant || relevant.has(edge.id)

  // A chosen config key roots the tree at the directives added under it.
  if (configKey) {
    const roots = [...tree.nodes.values()]
      .filter((node) => node.configKeys.includes(configKey))
      .map((node) => ({ id: node.id, via: '' }))
      .filter(keep)
    return (
      <TreeList
        empty="No directives were added under this config key."
        count={roots.length}
      >
        {roots.map((edge) => (
          <DirectiveTreeRow
            key={edge.id}
            tree={tree}
            edge={edge}
            depth={0}
            ancestors={[]}
            relevant={relevant}
            query={query}
          />
        ))}
      </TreeList>
    )
  }

  const unlinked = tree.unlinked.map((id) => ({ id, via: '' })).filter(keep)
  const sections: {
    key: string
    title: string
    groups: DirectiveOriginGroup[]
  }[] = [
    { key: 'controllers', title: 'Controller', groups: tree.controllers },
    { key: 'remote', title: 'Another bus', groups: tree.remote },
  ]
  const groups = sections.flatMap((section) =>
    section.groups
      .map((group) => ({
        key: `${section.key}:${group.label}`,
        kind: section.title,
        label: group.label,
        edges: group.children.filter(keep),
      }))
      .filter((group) => group.edges.length),
  )
  if (unlinked.length) {
    groups.push({
      key: 'unlinked',
      kind: 'No origin',
      label: 'added without an origin',
      edges: unlinked,
    })
  }

  return (
    <TreeList
      empty={query ? 'No directives match.' : 'No directives are active.'}
      count={groups.length}
    >
      {groups.map((group) => (
        <OriginGroupRow
          key={group.key}
          kind={group.kind}
          label={group.label}
          edges={group.edges}
          tree={tree}
          relevant={relevant}
          query={query}
        />
      ))}
    </TreeList>
  )
}

// TreeList renders the tree's top level, or the empty message.
function TreeList({
  empty,
  count,
  children,
}: {
  empty: string
  count: number
  children: ReactNode
}) {
  if (!count) {
    return <p className="text-foreground-alt/50 px-4 py-3 text-xs">{empty}</p>
  }
  return <ul className="divide-foreground/6 divide-y">{children}</ul>
}

// OriginGroupRow is one origin outside the directives, with the directives it added.
// A controller's Execute is one such origin.
function OriginGroupRow({
  kind,
  label,
  edges,
  tree,
  relevant,
  query,
}: {
  kind: string
  label: string
  edges: DirectiveEdge[]
  tree: DirectiveTree
  relevant: Set<number> | null
  query: string
}) {
  const [userOpen, setOpen] = useState(false)
  const open = userOpen || !!query

  return (
    <li>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="hover:bg-foreground/[0.03] focus-visible:bg-foreground/[0.04] flex w-full items-center gap-3 px-4 py-2 text-left text-xs outline-none"
      >
        <Chevron open={open} />
        <span className="text-foreground-alt/50 shrink-0">{kind}</span>
        <span className="text-foreground min-w-0 flex-1 truncate font-mono">
          {label}
        </span>
        <span className="text-foreground-alt/70 shrink-0 font-mono tabular-nums">
          {formatCount(edges.length)}
        </span>
      </button>
      {open && (
        <ul className="pb-1">
          {edges.map((edge) => (
            <DirectiveTreeRow
              key={`${edge.id}@${edge.via}`}
              tree={tree}
              edge={edge}
              depth={1}
              ancestors={[]}
              relevant={relevant}
              query={query}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

// DirectiveTreeRow is one directive under its origin.
// It expands to the directives it added.
// A filtered tree opens each row on the path to a match.
function DirectiveTreeRow({
  tree,
  edge,
  depth,
  ancestors,
  relevant,
  query,
}: {
  tree: DirectiveTree
  edge: DirectiveEdge
  depth: number
  ancestors: number[]
  relevant: Set<number> | null
  query: string
}) {
  const [userOpen, setOpen] = useState(false)
  const node = tree.nodes.get(edge.id)
  if (!node) {
    return null
  }
  const backLink = ancestors.includes(edge.id)
  const children = backLink
    ? []
    : node.children.filter((child) => !relevant || relevant.has(child.id))
  const expandable = children.length > 0
  const open =
    expandable && (userOpen || (!!query && !directiveMatches(node, query)))

  return (
    <li>
      <button
        type="button"
        aria-expanded={expandable ? open : undefined}
        disabled={!expandable}
        onClick={() => setOpen(!open)}
        style={{ '--tree-depth': depth }}
        className="hover:bg-foreground/[0.03] focus-visible:bg-foreground/[0.04] directive-tree-indent flex w-full items-center gap-3 py-1.5 pr-4 text-left text-xs outline-none disabled:cursor-default"
      >
        <DirectiveRowLabel
          node={node}
          via={edge.via}
          backLink={backLink}
          open={open}
          childCount={children.length}
        />
      </button>
      {open && (
        <ul>
          {children.map((child) => (
            <DirectiveTreeRow
              key={`${child.id}@${child.via}`}
              tree={tree}
              edge={child}
              depth={depth + 1}
              ancestors={[...ancestors, edge.id]}
              relevant={relevant}
              query={query}
            />
          ))}
        </ul>
      )}
    </li>
  )
}

// DirectiveRowLabel is the content of one directive row.
// It shows the marker, name, arguments, origin controller, config keys, and child count.
function DirectiveRowLabel({
  node,
  via,
  backLink,
  open,
  childCount,
}: {
  node: DirectiveNode
  via: string
  backLink: boolean
  open: boolean
  childCount: number
}) {
  const args = identArgs(node)
  return (
    <>
      {backLink ? (
        <LuCornerLeftUp
          className="text-foreground-alt/50 size-3.5 shrink-0"
          aria-label="Back link"
        />
      ) : (
        <Chevron open={open} hidden={!childCount} />
      )}
      <span className="text-foreground min-w-0 flex-1 truncate font-mono">
        {node.name}
        {args && <span className="text-foreground-alt/60"> {args}</span>}
      </span>
      {backLink && (
        <span className="text-foreground-alt/50 shrink-0">back link</span>
      )}
      {via && (
        <span className="text-foreground-alt/50 hidden max-w-60 shrink-0 truncate font-mono @2xl:block">
          via {via}
        </span>
      )}
      {node.configKeys.map((key) => (
        <span
          key={key}
          className="bg-brand/10 text-brand max-w-48 shrink-0 truncate rounded px-1.5 py-0.5 font-mono"
        >
          {key}
        </span>
      ))}
      {childCount > 0 && (
        <span className="text-foreground-alt/50 w-8 shrink-0 text-right font-mono tabular-nums">
          {formatCount(childCount)}
        </span>
      )}
    </>
  )
}

// identArgs returns the arguments of the directive's ident, or the whole ident
// when it is not the name followed by its arguments. It is empty when the
// ident is just the name.
function identArgs(node: { name: string; ident: string }): string {
  if (!node.ident || node.ident === node.name) {
    return ''
  }
  const prefix = node.name + '<'
  if (node.ident.startsWith(prefix) && node.ident.endsWith('>')) {
    return node.ident.slice(prefix.length, -1)
  }
  return node.ident
}

// Chevron is the expand marker, kept as a spacer for leaves.
function Chevron({ open, hidden }: { open: boolean; hidden?: boolean }) {
  return (
    <LuChevronRight
      className={cn(
        'text-foreground-alt/50 size-3.5 shrink-0 transition-transform duration-150',
        open && 'rotate-90',
        hidden && 'invisible',
      )}
      aria-hidden="true"
    />
  )
}
