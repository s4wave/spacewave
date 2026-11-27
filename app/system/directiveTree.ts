import type { WatchDirectivesResponse } from '@s4wave/sdk/status/status.pb.js'

// DirectiveEdge points from an origin to a directive it added.
// via is the origin controller, when the bus knows it.
export interface DirectiveEdge {
  id: number
  via: string
}

// DirectiveNode is one active directive and the directives it added.
export interface DirectiveNode {
  id: number
  name: string
  ident: string
  // configKeys are the ConfigSet keys the directive's references were added
  // under.
  configKeys: string[]
  children: DirectiveEdge[]
}

// DirectiveOriginGroup is the directives added by one origin outside this bus's directives.
// The origin is a controller's Execute or a directive on another bus.
export interface DirectiveOriginGroup {
  label: string
  children: DirectiveEdge[]
}

// DirectiveTree is the session bus's directives linked to their origins.
// The links form a graph.
// A shared directive has several origins.
// An origin can sit below the directive it added.
export interface DirectiveTree {
  nodes: Map<number, DirectiveNode>
  // controllers contains controller origins that added directives from
  // their Execute, outside any directive.
  controllers: DirectiveOriginGroup[]
  // remote contains groups of directives added by a directive on another bus.
  remote: DirectiveOriginGroup[]
  // unlinked are the directives whose references specify no origin at all.
  unlinked: number[]
  // configKeys are the distinct ConfigSet keys, sorted.
  configKeys: string[]
}

// buildDirectiveTree links each directive to its origins. A directive held only
// by weak references links through those, so an observed directive still has
// a place in the tree.
export function buildDirectiveTree(
  resp: WatchDirectivesResponse,
): DirectiveTree {
  const nodes = new Map<number, DirectiveNode>()
  for (const info of resp.directives ?? []) {
    if (!info.id) {
      continue
    }
    nodes.set(info.id, {
      id: info.id,
      name: info.name || 'unknown',
      ident: info.ident ?? '',
      configKeys: [],
      children: [],
    })
  }

  const controllers = new Map<string, DirectiveEdge[]>()
  const remote = new Map<string, DirectiveEdge[]>()
  const unlinked: number[] = []
  const configKeys = new Set<string>()
  const seen = new Set<string>()
  const add = (list: DirectiveEdge[], key: string, edge: DirectiveEdge) => {
    if (!seen.has(key)) {
      seen.add(key)
      list.push(edge)
    }
  }

  for (const info of resp.directives ?? []) {
    const node = info.id ? nodes.get(info.id) : undefined
    if (!node) {
      continue
    }
    const refs = info.references ?? []
    const strong = refs.filter((ref) => !ref.weak)
    const nodeKeys = new Set<string>()
    let linked = false
    for (const ref of strong.length ? strong : refs) {
      const via = ref.originControllerId ?? ''
      const edge = { id: node.id, via }
      if (ref.configKey) {
        nodeKeys.add(ref.configKey)
        configKeys.add(ref.configKey)
      }
      const parent = ref.originDirectiveId
        ? nodes.get(ref.originDirectiveId)
        : undefined
      if (parent) {
        add(parent.children, `d${parent.id}>${node.id}@${via}`, edge)
      } else if (ref.originDirectiveIdent) {
        const list = groupList(remote, ref.originDirectiveIdent)
        add(list, `r${ref.originDirectiveIdent}>${node.id}@${via}`, edge)
      } else if (via) {
        add(groupList(controllers, via), `c${via}>${node.id}`, edge)
      } else {
        continue
      }
      linked = true
    }
    node.configKeys = [...nodeKeys]
    if (!linked) {
      unlinked.push(node.id)
    }
  }

  return {
    nodes,
    controllers: sortedGroups(controllers),
    remote: sortedGroups(remote),
    unlinked,
    configKeys: [...configKeys].sort(),
  }
}

// relevantDirectives returns the directives matching query plus every
// directive that can reach one of them, so a filtered tree keeps the paths to
// each match. It returns null for an empty query.
export function relevantDirectives(
  tree: DirectiveTree,
  query: string,
): Set<number> | null {
  if (!query) {
    return null
  }
  const parents = new Map<number, number[]>()
  for (const node of tree.nodes.values()) {
    for (const child of node.children) {
      const list = parents.get(child.id)
      if (list) {
        list.push(node.id)
      } else {
        parents.set(child.id, [node.id])
      }
    }
  }
  const relevant = new Set<number>()
  const queue: number[] = []
  for (const node of tree.nodes.values()) {
    if (directiveMatches(node, query)) {
      relevant.add(node.id)
      queue.push(node.id)
    }
  }
  while (queue.length) {
    for (const parent of parents.get(queue.pop()!) ?? []) {
      if (!relevant.has(parent)) {
        relevant.add(parent)
        queue.push(parent)
      }
    }
  }
  return relevant
}

// directiveMatches reports whether the node's name, identity, or config keys
// contain the lowercase query.
export function directiveMatches(node: DirectiveNode, query: string): boolean {
  return [node.name, node.ident, ...node.configKeys].some((field) =>
    field.toLowerCase().includes(query),
  )
}

// groupList returns the edge list for key, creating it if needed.
function groupList(
  groups: Map<string, DirectiveEdge[]>,
  key: string,
): DirectiveEdge[] {
  let list = groups.get(key)
  if (!list) {
    list = []
    groups.set(key, list)
  }
  return list
}

// sortedGroups orders origin groups by label.
function sortedGroups(
  groups: Map<string, DirectiveEdge[]>,
): DirectiveOriginGroup[] {
  return [...groups]
    .map(([label, children]) => ({ label, children }))
    .sort((a, b) => a.label.localeCompare(b.label))
}
