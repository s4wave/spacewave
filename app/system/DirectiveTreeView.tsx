import { utilityState as listUtilityState } from '@s4wave/web/style/tokens.stylex.js'
import { useMemo, useState, type ReactNode } from 'react'
import { LuChevronRight, LuCornerLeftUp } from '@s4wave/web/images/icons.js'

import {
  directiveMatches,
  relevantDirectives,
  type DirectiveOriginGroup,
  type DirectiveEdge,
  type DirectiveNode,
  type DirectiveTree,
} from './directiveTree.js'
import { formatCount } from './format.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    '--tw-outline-style': 'none',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-foreground']} 4%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    textAlign: 'left',
    width: '100%',
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
  },
  rule4: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    flex: 1,
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    minWidth: 0,
  },
  rule5: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule6: { paddingBottom: themeLayout['--spacing'] },
  rule7: {
    '--tw-outline-style': 'none',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-foreground']} 4%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 3%, transparent)`,
      },
    },
    cursor: { default: null, ':disabled': 'default' },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    outlineStyle: 'none',
    textAlign: 'left',
    paddingLeft: 'calc(1rem + var(--tree-depth) * 1.25rem)',
    paddingRight: `calc(${themeLayout['--spacing']} * 4)`,
    width: '100%',
  },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule9: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    overflow: 'hidden',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: { default: 'none', '@container (min-width: 42rem)': 'block' },
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 60)`,
  },
  rule10: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderRadius: '.25rem',
    overflow: 'hidden',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    textOverflow: 'ellipsis',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 48)`,
  },
  rule11: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    fontFamily: themeFonts['--font-mono'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    '--tw-duration': '.15s',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    flexShrink: 0,
    transitionDuration: '.15s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: { rotate: '90deg' },
  rule4: { visibility: 'hidden' },
})

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
        {(childSx) => (
          <>
            {roots.map((edge) => (
              <DirectiveTreeRow
                sx={childSx}
                key={edge.id}
                tree={tree}
                edge={edge}
                depth={0}
                ancestors={[]}
                relevant={relevant}
                query={query}
              />
            ))}
          </>
        )}
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
      {(childSx) => (
        <>
          {groups.map((group) => (
            <OriginGroupRow
              sx={childSx}
              key={group.key}
              kind={group.kind}
              label={group.label}
              edges={group.edges}
              tree={tree}
              relevant={relevant}
              query={query}
            />
          ))}
        </>
      )}
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
  children: (childSx: stylex.StyleXStyles) => ReactNode
}) {
  if (!count) {
    return <p sx={stylexStyles2.rule1}>{empty}</p>
  }
  return <ul sx={false}>{children?.(slotChildren.child)}</ul>
}

// OriginGroupRow is one origin outside the directives, with the directives it added.
// A controller's Execute is one such origin.
function OriginGroupRow({
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const [userOpen, setOpen] = useState(false)
  const open = userOpen || !!query

  return (
    <li sx={sx}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        sx={stylexStyles2.rule2}
      >
        <Chevron open={open} />
        <span sx={stylexStyles2.rule3}>{kind}</span>
        <span sx={stylexStyles2.rule4}>{label}</span>
        <span sx={stylexStyles2.rule5}>{formatCount(edges.length)}</span>
      </button>
      {open && (
        <ul sx={stylexStyles2.rule6}>
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
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
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
    <li sx={sx}>
      <button
        type="button"
        aria-expanded={expandable ? open : undefined}
        disabled={!expandable}
        onClick={() => setOpen(!open)}
        style={{ '--tree-depth': depth }}
        sx={stylexStyles2.rule7}
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
        <LuCornerLeftUp sx={stylexStyles.rule1} aria-label="Back link" />
      ) : (
        <Chevron open={open} hidden={!childCount} />
      )}
      <span sx={stylexStyles2.rule4}>
        {node.name}
        {args && <span sx={stylexStyles2.rule8}> {args}</span>}
      </span>
      {backLink && <span sx={stylexStyles2.rule3}>back link</span>}
      {via && <span sx={stylexStyles2.rule9}>via {via}</span>}
      {node.configKeys.map((key) => (
        <span key={key} sx={stylexStyles2.rule10}>
          {key}
        </span>
      ))}
      {childCount > 0 && (
        <span sx={stylexStyles2.rule11}>{formatCount(childCount)}</span>
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
      sx={[
        stylexStyles.rule2,
        !!open && stylexStyles.rule3,
        !!hidden && stylexStyles.rule4,
      ]}
      aria-hidden="true"
    />
  )
}

// slotChildren keeps the optional action in the original physical child sequence.
const slotChildren = stylex.create({
  child: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 6%, transparent)',
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': listUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': listUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})
