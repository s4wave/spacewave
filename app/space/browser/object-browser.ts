import type {
  GraphEdgeBucket,
  ObjectMeasure,
  ObjectMetadata,
  TypeCountEntry,
} from '@s4wave/sdk/world/world.pb.js'
import { iriToKey, isIRI } from '@s4wave/sdk/world/graph-utils.js'
import { TypePred } from '@s4wave/sdk/world/types/types.js'
import { getObjectDisplayName } from '@s4wave/web/space/object-tree.js'

// The object browser's model: navigation, the actions each state shows, and
// the relations of one object grouped for the graph and the list. Shown and
// Hint mirror ObjectBrowser.tla, so the screen offers an action only in the
// states where it works.

// ListNav is a list of objects to open: a type's objects, search results or
// the key tree.
export type ListNav =
  | { view: 'objects'; typeId: string }
  | { view: 'search'; query: string }
  | { view: 'keys' }

// BrowserNav is what the browser shows. An object remembers the list it was
// opened from, so Back after following relations returns to that list.
export type BrowserNav =
  | { view: 'types' }
  | ListNav
  | { view: 'object'; objectKey: string; list: ListNav }

// RelationsView is how an object's relations show.
export type RelationsView = 'graph' | 'list'

// BrowserState is the browser's navigation and the relations view a person
// keeps across objects.
export interface BrowserState {
  nav: BrowserNav
  rel: RelationsView
}

// BrowserEvent is a person's press. openObject from an object follows a
// relation and keeps the list Back returns to.
export type BrowserEvent =
  | { type: 'openType'; typeId: string }
  | { type: 'search'; query: string }
  | { type: 'openKeys' }
  | { type: 'openObject'; objectKey: string }
  | { type: 'back' }
  | { type: 'toggleRelations' }

export const initialBrowserState: BrowserState = {
  nav: { view: 'types' },
  rel: 'graph',
}

// browserReducer moves the browser on a press.
export function browserReducer(
  state: BrowserState,
  event: BrowserEvent,
): BrowserState {
  const { nav } = state
  switch (event.type) {
    case 'openType':
      return { ...state, nav: { view: 'objects', typeId: event.typeId } }
    case 'search':
      return {
        ...state,
        nav: event.query.trim()
          ? { view: 'search', query: event.query }
          : { view: 'types' },
      }
    case 'openKeys':
      return { ...state, nav: { view: 'keys' } }
    case 'openObject': {
      if (nav.view === 'types') return state
      const list = nav.view === 'object' ? nav.list : nav
      return {
        ...state,
        nav: { view: 'object', objectKey: event.objectKey, list },
      }
    }
    case 'back':
      if (nav.view === 'types') return state
      return {
        ...state,
        nav: nav.view === 'object' ? nav.list : { view: 'types' },
      }
    case 'toggleRelations':
      return { ...state, rel: state.rel === 'graph' ? 'list' : 'graph' }
  }
}

// BrowserAction is a button of ObjectBrowser.tla.
export type BrowserAction =
  | 'openType'
  | 'openObject'
  | 'follow'
  | 'back'
  | 'search'
  | 'openKeys'
  | 'retry'
  | 'copyKey'
  | 'open'
  | 'toggleRelations'

// BrowserScreen is the state ObjectBrowser.tla charts: the view, the read
// behind it, the connection, whether the open object still exists, and how
// many visible objects the World holds.
export interface BrowserScreen {
  view: BrowserNav['view']
  load: 'loading' | 'ready' | 'failed'
  conn: 'up' | 'down'
  target: 'present' | 'deleted'
  count: number
}

// shownActions is the spec's Shown: the actions on screen, from the state
// alone.
export function shownActions(s: BrowserScreen): ReadonlySet<BrowserAction> {
  const nav: BrowserAction[] = s.view === 'types' ? [] : ['back']
  const online = s.conn === 'up'
  if (s.load === 'loading') return new Set(nav)
  if (s.load === 'failed')
    return new Set([...nav, ...(online ? (['retry'] as const) : [])])
  if (!online) {
    const object = s.view === 'object' && s.target === 'present'
    return new Set([
      ...nav,
      ...(object ? (['copyKey', 'open', 'toggleRelations'] as const) : []),
    ])
  }
  if (s.view === 'types') {
    return new Set<BrowserAction>([
      ...(s.count > 0 ? (['openType'] as const) : []),
      'search',
      'openKeys',
    ])
  }
  if (s.view !== 'object') {
    return new Set<BrowserAction>([
      ...nav,
      ...(s.count > 0 ? (['openObject'] as const) : []),
      ...(s.view === 'search' ? (['search'] as const) : []),
    ])
  }
  if (s.target === 'deleted') return new Set(nav)
  return new Set<BrowserAction>([
    ...nav,
    'follow',
    'copyKey',
    'open',
    'toggleRelations',
  ])
}

// browserHint is the spec's Hint: what the screen says when a person has
// nothing to press, or why an action is missing.
export function browserHint(s: BrowserScreen): string {
  if (s.load === 'loading') return 'Loading'
  if (s.conn === 'down') return 'Offline: reconnecting'
  if (s.load === 'failed') return 'Could not load'
  if (s.view === 'object' && s.target === 'deleted')
    return 'Deleted by another writer'
  if (s.count === 0) return 'This Space has no objects yet'
  return ''
}

// TypeCounts maps each present type to its count entry.
export type TypeCounts = ReadonlyMap<string, TypeCountEntry>

// applyTypeCounts applies one watch response to counts: a zero count removes
// the type, any other count replaces it.
export function applyTypeCounts(
  counts: TypeCounts,
  entries: readonly TypeCountEntry[],
): TypeCounts {
  const next = new Map(counts)
  for (const entry of entries) {
    const typeId = entry.typeId ?? ''
    if (!entry.count) {
      next.delete(typeId)
    } else {
      next.set(typeId, entry)
    }
  }
  return next
}

// visibleTypeCounts lists the types that are not internal, and the total of
// their objects.
export function visibleTypeCounts(counts: TypeCounts): {
  types: TypeCountEntry[]
  total: number
} {
  const types = Array.from(counts.values()).filter((entry) => !entry.internal)
  const total = types.reduce((sum, entry) => sum + Number(entry.count ?? 0n), 0)
  return { types, total }
}

// RelationEnd is the object at the far end of a relation.
export interface RelationEnd {
  objectKey: string
  typeId: string
  title: string
  // state is what the object's type declares as its state, empty for none.
  state: string
  // measures are the object's summable quantities, such as a receipt's tokens.
  measures: readonly ObjectMeasure[]
}

// RelationGroup is the relations of one direction and predicate to objects of
// one type. A group of more than one member is a family, which the graph draws
// as one counted node.
export interface RelationGroup {
  id: string
  direction: 'out' | 'in'
  predicate: string
  label: string
  typeId: string
  members: RelationEnd[]
}

// relationEndKeys lists the keys at the far end of the bucket's relations,
// each once, for one metadata read.
export function relationEndKeys(bucket: GraphEdgeBucket | undefined): string[] {
  const keys = new Set<string>()
  for (const { iri } of bucketEnds(bucket)) keys.add(iriToKey(iri))
  return Array.from(keys)
}

// groupRelations groups the bucket's relations by direction, predicate and
// far-end type: outgoing first, each direction by label. Type edges and
// literal values are not relations to objects and are left out.
export function groupRelations(
  bucket: GraphEdgeBucket | undefined,
  metadata: ReadonlyMap<string, ObjectMetadata>,
): RelationGroup[] {
  const groups = new Map<string, RelationGroup>()
  for (const { direction, predicate, iri } of bucketEnds(bucket)) {
    const objectKey = iriToKey(iri)
    const md = metadata.get(objectKey)
    const typeId = md?.typeId ?? ''
    const id = direction + ' ' + predicate + ' ' + typeId
    let group = groups.get(id)
    if (!group) {
      group = {
        id,
        direction,
        predicate,
        label: predicateLabel(predicate),
        typeId,
        members: [],
      }
      groups.set(id, group)
    }
    if (!group.members.some((m) => m.objectKey === objectKey)) {
      group.members.push({
        objectKey,
        typeId,
        title: md?.declaration?.title || '',
        state: md?.declaration?.state || '',
        measures: md?.declaration?.measures ?? [],
      })
    }
  }
  return Array.from(groups.values()).toSorted(
    (a, b) =>
      (a.direction === b.direction ? 0 : a.direction === 'out' ? -1 : 1) ||
      a.label.localeCompare(b.label) ||
      a.typeId.localeCompare(b.typeId),
  )
}

// stateSummary sums a family's member states, most common first, such as
// "3 running, 2 passed", or empty when no member declares a state.
export function stateSummary(members: readonly RelationEnd[]): string {
  const counts = new Map<string, number>()
  for (const { state } of members) {
    if (state) counts.set(state, (counts.get(state) ?? 0) + 1)
  }
  return Array.from(counts)
    .toSorted((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([state, n]) => n + ' ' + state)
    .join(', ')
}

// measureSummary sums a family's member measures by name, in the order the
// members first declare them, such as "$3.18, 1.9 M tokens", or empty when no
// member declares a measure.
export function measureSummary(members: readonly RelationEnd[]): string {
  const sums = new Map<string, { unit: string; value: number }>()
  for (const { measures } of members) {
    for (const { name = '', unit = '', value = 0 } of measures) {
      const sum = sums.get(name) ?? { unit, value: 0 }
      sum.value += value
      sums.set(name, sum)
    }
  }
  return Array.from(sums, ([name, { unit, value }]) =>
    formatMeasure(name, unit, value),
  ).join(', ')
}

// formatMeasure writes one summed measure: dollars for the usd unit, else the
// amount with a K or M suffix and its unit or name, such as "1.9 M tokens".
export function formatMeasure(
  name: string,
  unit: string,
  value: number,
): string {
  if (unit === 'usd') return '$' + value.toFixed(2)
  const scaled =
    value >= 1e6
      ? (value / 1e6).toFixed(1) + ' M'
      : value >= 1e4
        ? (value / 1e3).toFixed(0) + ' K'
        : String(Math.round(value * 100) / 100)
  return scaled + ' ' + (unit || name)
}

// bucketEnds yields each relation of the bucket as its direction, predicate
// and far-end IRI.
function* bucketEnds(bucket: GraphEdgeBucket | undefined) {
  for (const quad of bucket?.outgoing ?? []) {
    if (quad.predicate === TypePred || !isIRI(quad.obj)) continue
    yield {
      direction: 'out' as const,
      predicate: quad.predicate ?? '',
      iri: quad.obj,
    }
  }
  for (const quad of bucket?.incoming ?? []) {
    if (quad.predicate === TypePred || !isIRI(quad.subject)) continue
    yield {
      direction: 'in' as const,
      predicate: quad.predicate ?? '',
      iri: quad.subject,
    }
  }
}

// predicateLabel names a predicate in words: the last segment of its IRI,
// such as "pursues" for <glados/pursues>, with dashes and underscores as
// spaces.
export function predicateLabel(predicate: string): string {
  const bare = isIRI(predicate) ? predicate.slice(1, -1) : predicate
  const segment = bare.split('/').filter(Boolean).at(-1) ?? bare
  return segment.replace(/[-_]+/g, ' ').trim() || 'related'
}

// GraphNode is one box of the relations graph: the open object, one related
// object, a family, or the relations past what the graph draws.
export interface GraphNode {
  id: string
  kind: 'center' | 'object' | 'family' | 'more'
  x: number
  y: number
  width: number
  title: string
  subtitle: string
  // objectKey is the object a single node opens.
  objectKey?: string
  // edge is the label of the edge to the center, absent on the center.
  edge?: { label: string; direction: 'out' | 'in'; x: number; y: number }
}

// graphSize is the relations graph's drawing size, as the mock-up sets it.
export const graphSize = {
  width: 560,
  height: 470,
  nodeHeight: 46,
  centerHeight: 50,
} as const

// graphSlots bounds the nodes of each half: outgoing on the upper arc and the
// sides, incoming on the lower row. Past them, one node counts the rest.
const graphSlots = { out: 5, in: 3 } as const

// nodeTitleChars bounds a node's title before it ends in an ellipsis.
const nodeTitleChars = 22

// layoutRelationGraph places the open object at the center, outgoing groups
// on the upper arc from left to right and incoming groups on a row below it.
// Each group is one node: an object, or a family with its count. Groups past
// a half's slots fold into one node that opens the list.
export function layoutRelationGraph(
  center: { title: string; subtitle: string },
  groups: readonly RelationGroup[],
  nameType: (typeId: string, plural: boolean) => string,
): GraphNode[] {
  const cx = graphSize.width / 2
  const cy = graphSize.height / 2
  const nodes: GraphNode[] = [
    {
      id: 'center',
      kind: 'center',
      x: cx,
      y: cy,
      width: nodeWidth(center.title, 190),
      title: clip(center.title),
      subtitle: center.subtitle,
    },
  ]
  for (const direction of ['out', 'in'] as const) {
    const half = groups.filter((g) => g.direction === direction)
    const slots = graphSlots[direction]
    const shown = half.length > slots ? half.slice(0, slots - 1) : half
    const rest = half.slice(shown.length)
    const n = shown.length + (rest.length ? 1 : 0)
    const angles = upperAngles(n)
    const place = (i: number) =>
      direction === 'out'
        ? {
            x: Math.round(cx + Math.cos(angles[i]) * 190),
            y: Math.round(cy - Math.sin(angles[i]) * 175),
          }
        : { x: cx + (i - (n - 1) / 2) * lowerStep, y: cy + 170 }
    shown.forEach((group, i) => {
      const { x, y } = place(i)
      const single = group.members.length === 1 ? group.members[0] : undefined
      const title = single
        ? single.title || getObjectDisplayName(single.objectKey)
        : group.members.length + ' ' + nameType(group.typeId, true)
      const node: GraphNode = {
        id: group.id,
        kind: single ? 'object' : 'family',
        x,
        y,
        width: nodeWidth(title, direction === 'out' ? 180 : lowerStep - 10),
        title: clip(title),
        subtitle: nameType(group.typeId, false),
        objectKey: single?.objectKey,
      }
      nodes.push({
        ...node,
        edge: {
          label: group.label,
          direction,
          ...edgeLabelAt(nodes[0], node, group.label),
        },
      })
    })
    if (rest.length) {
      const { x, y } = place(shown.length)
      const count = rest.reduce((sum, g) => sum + g.members.length, 0)
      const title = count + ' more'
      nodes.push({
        id: direction + ' more',
        kind: 'more',
        x,
        y,
        width: nodeWidth(title, 150),
        title,
        subtitle: rest.length + ' relations',
        edge: { label: '', direction, x, y },
      })
    }
  }
  return nodes
}

// edgeLabelAt places an edge's label at the middle of the line between the
// two boxes when the line is long enough to hold it, and otherwise just above
// the related node, as on a side node that sits close beside the center.
function edgeLabelAt(
  center: GraphNode,
  node: GraphNode,
  label: string,
): { x: number; y: number } {
  const a = boxEdge(center, node.x, node.y)
  const b = boxEdge(node, center.x, center.y)
  if (Math.hypot(b.x - a.x, b.y - a.y) >= label.length * 6.5 + 16) {
    return { x: Math.round((a.x + b.x) / 2), y: Math.round((a.y + b.y) / 2) }
  }
  return { x: node.x, y: node.y - nodeHeight(node) / 2 - 9 }
}

// nodeHeight is a node's box height.
export function nodeHeight(node: GraphNode): number {
  return node.kind === 'center' ? graphSize.centerHeight : graphSize.nodeHeight
}

// boxEdge is where the line from (x, y) toward the node's center meets the
// node's border.
export function boxEdge(
  node: GraphNode,
  x: number,
  y: number,
): { x: number; y: number } {
  const dx = x - node.x
  const dy = y - node.y
  const hw = node.width / 2
  const hh = nodeHeight(node) / 2
  const t = Math.min(
    dx ? hw / Math.abs(dx) : Infinity,
    dy ? hh / Math.abs(dy) : Infinity,
    1,
  )
  return { x: node.x + dx * t, y: node.y + dy * t }
}

// upperAngles spreads n nodes over the upper half from left to right, the
// first and last on the sides.
function upperAngles(n: number): number[] {
  if (n === 1) return [Math.PI / 2]
  return Array.from({ length: n }, (_, i) => Math.PI - (i * Math.PI) / (n - 1))
}

// lowerStep spaces the incoming row; its nodes are at most lowerStep - 10
// wide, so neighbours keep a gap and three fit the drawing.
const lowerStep = 180

// nodeWidth fits a node to its clipped title, at most max wide.
function nodeWidth(title: string, max: number): number {
  return Math.min(max, Math.max(110, clip(title).length * 7.5 + 28))
}

// clip ends a title past nodeTitleChars in an ellipsis.
function clip(title: string): string {
  return title.length > nodeTitleChars
    ? title.slice(0, nodeTitleChars - 1) + '\u2026'
    : title
}
