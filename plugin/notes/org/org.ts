import { parse } from 'orga'

export interface OrgLocation {
  offset: number
}

export interface OrgPosition {
  start: OrgLocation
  end: OrgLocation
}

export interface OrgNode {
  id: string
  type: string
  position: OrgPosition
  source: string
  children: OrgNode[]
  keyword?: string
  level?: number
  tags?: string[]
  title?: string
}

export interface OrgDocument {
  source: string
  root: OrgNode
  edits: ReadonlyMap<string, string>
}

export interface OrgHeadingUpdate {
  keyword?: string
  title: string
  tags?: string[]
}

export interface OrgBlockUpdate {
  name: string
  params?: string[]
  value: string
}

export function parseOrg(source: string): OrgDocument {
  const parsed: unknown = parse(source)
  const root = toOrgNode(parsed, source, '0')

  if (!root) {
    return {
      source,
      root: {
        id: '0',
        type: 'document',
        position: { start: { offset: 0 }, end: { offset: source.length } },
        source,
        children: [],
      },
      edits: new Map(),
    }
  }

  return { source, root, edits: new Map() }
}

export function serializeOrg(document: OrgDocument): string {
  if (document.edits.size === 0) {
    return document.source
  }

  const spans = [...document.edits.entries()]
    .map(([id, replacement]) => {
      const node = findNode(document.root, id)
      if (!node) {
        throw new Error(`Org node ${id} does not exist`)
      }
      return {
        start: node.position.start.offset,
        end: node.position.end.offset,
        replacement,
      }
    })
    .sort((left, right) => left.start - right.start)

  let output = ''
  let cursor = 0
  for (const span of spans) {
    if (span.start < cursor) {
      throw new Error('Org edits must not overlap')
    }
    output += document.source.slice(cursor, span.start)
    output += span.replacement
    cursor = span.end
  }
  output += document.source.slice(cursor)
  return output
}

export function findFirstOrgNode(document: OrgDocument, type: string): OrgNode {
  const node = findFirstNode(document.root, type)
  if (!node) {
    throw new Error(`Org node type ${type} does not exist`)
  }
  return node
}

export function updateOrgHeading(
  document: OrgDocument,
  nodeId: string,
  update: OrgHeadingUpdate,
): OrgDocument {
  const node = findNode(document.root, nodeId)
  if (!node) {
    throw new Error(`Org heading ${nodeId} does not exist`)
  }
  if (node.type !== 'headline') {
    throw new Error(`Org node ${nodeId} is ${node.type}, not headline`)
  }

  const edits = new Map(document.edits)
  edits.set(nodeId, emitOrgHeading(document.source, node, update))
  return { ...document, edits }
}

export function updateOrgBlock(
  document: OrgDocument,
  nodeId: string,
  update: OrgBlockUpdate,
): OrgDocument {
  const node = findNode(document.root, nodeId)
  if (!node) {
    throw new Error(`Org block ${nodeId} does not exist`)
  }
  if (node.type !== 'block') {
    throw new Error(`Org node ${nodeId} is ${node.type}, not block`)
  }

  const edits = new Map(document.edits)
  edits.set(nodeId, emitOrgBlock(document.source, node, update))
  return { ...document, edits }
}

export function emitOrgBlock(
  source: string,
  node: OrgNode,
  update: OrgBlockUpdate,
): string {
  if (node.type !== 'block') {
    throw new Error(`Org node ${node.id} is ${node.type}, not block`)
  }

  const params =
    update.params && update.params.length > 0
      ? ` ${update.params.join(' ')}`
      : ''
  const value = update.value.endsWith('\n') ? update.value : `${update.value}\n`
  const original = source.slice(
    node.position.start.offset,
    node.position.end.offset,
  )
  const newline = original.endsWith('\n') ? '\n' : ''

  return `#+begin_${update.name}${params}\n${value}#+end_${update.name}${newline}`
}

export function emitOrgHeading(
  source: string,
  node: OrgNode,
  update: OrgHeadingUpdate,
): string {
  if (node.type !== 'headline') {
    throw new Error(`Org node ${node.id} is ${node.type}, not headline`)
  }

  const level = node.level ?? 1
  const keyword = update.keyword ? ` ${update.keyword}` : ''
  const tags =
    update.tags && update.tags.length > 0 ? ` :${update.tags.join(':')}:` : ''
  const original = source.slice(
    node.position.start.offset,
    node.position.end.offset,
  )
  const newline = original.endsWith('\n') ? '\n' : ''

  return `${'*'.repeat(level)}${keyword} ${update.title}${tags}${newline}`
}

function toOrgNode(value: unknown, source: string, id: string): OrgNode | null {
  const object = objectRecord(value)
  if (!object) {
    return null
  }

  const type = stringField(object, 'type')
  const position = positionField(object, source.length)
  if (!type || !position) {
    return null
  }

  const childrenValue = object.children
  const children = Array.isArray(childrenValue)
    ? childrenValue.flatMap((child, index) => {
        const node = toOrgNode(child, source, `${id}.${index}`)
        return node ? [node] : []
      })
    : []

  const titleParts: string[] = []
  for (const child of children) {
    if (child.type === 'text') {
      titleParts.push(child.source)
    }
  }
  const title = titleParts.join(' ').trim()

  return {
    id,
    type,
    position,
    source: source.slice(position.start.offset, position.end.offset),
    children,
    keyword: stringField(object, 'keyword'),
    level: numberField(object, 'level'),
    tags: stringArrayField(object, 'tags'),
    title: title.length > 0 ? title : undefined,
  }
}

function objectRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object') {
    return null
  }
  return value as Record<string, unknown>
}

function stringField(
  object: Record<string, unknown>,
  field: string,
): string | undefined {
  const value = object[field]
  return typeof value === 'string' ? value : undefined
}

function numberField(
  object: Record<string, unknown>,
  field: string,
): number | undefined {
  const value = object[field]
  return typeof value === 'number' ? value : undefined
}

function stringArrayField(
  object: Record<string, unknown>,
  field: string,
): string[] | undefined {
  const value = object[field]
  if (!Array.isArray(value)) {
    return undefined
  }
  const strings = value.filter((item) => typeof item === 'string')
  return strings.length === value.length ? strings : undefined
}

function positionField(
  object: Record<string, unknown>,
  sourceLength: number,
): OrgPosition | undefined {
  const position = objectRecord(object.position)
  if (!position) {
    return stringField(object, 'type') === 'document'
      ? { start: { offset: 0 }, end: { offset: sourceLength } }
      : undefined
  }

  const start = objectRecord(position.start)
  const end = objectRecord(position.end)
  const startOffset = start ? numberField(start, 'offset') : undefined
  const endOffset = end ? numberField(end, 'offset') : undefined

  if (startOffset === undefined || endOffset === undefined) {
    return undefined
  }

  return { start: { offset: startOffset }, end: { offset: endOffset } }
}

function findNode(node: OrgNode, id: string): OrgNode | undefined {
  if (node.id === id) {
    return node
  }
  for (const child of node.children) {
    const found = findNode(child, id)
    if (found) {
      return found
    }
  }
  return undefined
}

function findFirstNode(node: OrgNode, type: string): OrgNode | undefined {
  if (node.type === type) {
    return node
  }
  for (const child of node.children) {
    const found = findFirstNode(child, type)
    if (found) {
      return found
    }
  }
  return undefined
}
