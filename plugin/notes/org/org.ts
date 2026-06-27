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

export type OrgBridgeModeledKind =
  | 'headline'
  | 'paragraph'
  | 'block'
  | 'table'
  | 'list'

export type OrgBridgeSegment =
  | {
      kind: 'modeled'
      modeledKind: OrgBridgeModeledKind
      node: OrgNode
      source: string
    }
  | {
      kind: 'passthrough'
      node?: OrgNode
      source: string
    }

export interface OrgMetadataSplit {
  metadata: string
  body: string
}

export interface OrgHeadingParts {
  level: number
  text: string
}

export interface OrgBlockParts {
  name: string
  params: string[]
  value: string
}

export interface OrgTableParts {
  rows: string[][]
}

export interface OrgListParts {
  ordered: boolean
  items: string[]
}

export interface OrgLinkParts {
  url: string
  text: string
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

export function splitOrgMetadata(source: string): OrgMetadataSplit {
  const document = parseOrg(source)
  let sawKeyword = false
  let bodyStart = 0
  let foundBody = false

  for (const child of document.root.children) {
    if (!sawKeyword) {
      if (child.type !== 'keyword') {
        break
      }
      sawKeyword = true
      continue
    }

    if (child.type === 'keyword' || child.type === 'emptyLine') {
      continue
    }

    bodyStart = child.position.start.offset
    foundBody = true
    break
  }

  if (!sawKeyword) {
    return { metadata: '', body: source }
  }

  if (!foundBody) {
    bodyStart = source.length
  }

  return {
    metadata: source.slice(0, bodyStart),
    body: source.slice(bodyStart),
  }
}

export function reassembleOrgMetadata(metadata: string, body: string): string {
  return `${metadata}${body}`
}

export function getOrgBridgeSegments(
  document: OrgDocument,
): OrgBridgeSegment[] {
  const nodes = collectOrgBridgeNodes(document.root).sort(
    (left, right) => left.position.start.offset - right.position.start.offset,
  )
  const segments: OrgBridgeSegment[] = []
  let cursor = 0

  for (const node of nodes) {
    const start = node.position.start.offset
    const end = node.position.end.offset
    if (end <= cursor) {
      continue
    }

    if (start > cursor) {
      segments.push({
        kind: 'passthrough',
        source: document.source.slice(cursor, start),
      })
    }

    const source = document.source.slice(start, end)
    const modeledKind = classifyOrgBridgeNode(node)
    segments.push(
      modeledKind
        ? { kind: 'modeled', modeledKind, node, source }
        : { kind: 'passthrough', node, source },
    )
    cursor = end
  }

  if (cursor < document.source.length) {
    segments.push({
      kind: 'passthrough',
      source: document.source.slice(cursor),
    })
  }

  return segments.filter((segment) => segment.source.length > 0)
}

export function classifyOrgBridgeNode(
  node: OrgNode,
): OrgBridgeModeledKind | null {
  if (node.type === 'headline') {
    return 'headline'
  }
  if (node.type === 'paragraph') {
    return 'paragraph'
  }
  if (node.type === 'table' && readOrgTable(node)) {
    return 'table'
  }
  if (node.type === 'list' && readOrgList(node)) {
    return 'list'
  }
  if (node.type === 'block') {
    const block = readOrgBlock(node)
    if (block && isModeledOrgBlockName(block.name)) {
      return 'block'
    }
  }
  return null
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

export function readOrgHeading(node: OrgNode): OrgHeadingParts | null {
  if (node.type !== 'headline') {
    return null
  }

  const level = node.level ?? 1
  const textParts: string[] = []
  if (node.keyword) {
    textParts.push(node.keyword)
  }
  if (node.title) {
    textParts.push(node.title)
  }
  if (node.tags && node.tags.length > 0) {
    textParts.push(`:${node.tags.join(':')}:`)
  }

  const text =
    textParts.length > 0 ? textParts.join(' ') : headingTextFromSource(node)
  return { level, text }
}

export function emitOrgHeadingText(level: number, text: string): string {
  const stars = '*'.repeat(Math.max(1, Math.min(6, level)))
  return `${stars} ${text.trim()}\n`
}

export function readOrgBlock(node: OrgNode): OrgBlockParts | null {
  if (node.type !== 'block') {
    return null
  }

  const lines = trimFinalNewline(node.source).split('\n')
  if (lines.length < 2) {
    return null
  }

  const match = /^#\+begin_([^\s]+)(?:\s+(.*))?$/i.exec(lines[0])
  if (!match) {
    return null
  }

  const name = match[1].toLowerCase()
  const params =
    match[2]?.split(/\s+/).filter((param) => param.length > 0) ?? []
  const endIndex = lines.findLastIndex(
    (line) => line.toLowerCase() === `#+end_${name}`,
  )
  if (endIndex < 1) {
    return null
  }

  return {
    name,
    params,
    value: lines.slice(1, endIndex).join('\n'),
  }
}

export function emitOrgBlockText(
  name: string,
  params: string[],
  value: string,
): string {
  const paramText = params.length > 0 ? ` ${params.join(' ')}` : ''
  const body = value.endsWith('\n') ? value : `${value}\n`
  return `#+begin_${name}${paramText}\n${body}#+end_${name}\n`
}

export function readOrgTable(node: OrgNode): OrgTableParts | null {
  if (node.type !== 'table') {
    return null
  }

  const rows = node.children
    .filter((child) => child.type === 'table.row')
    .map((row) =>
      row.children
        .filter((child) => child.type === 'table.cell')
        .map((cell) => orgTextContent(cell).trim()),
    )
    .filter((row) => row.length > 0)

  return rows.length > 0 ? { rows } : null
}

export function emitOrgTable(rows: string[][]): string {
  return rows.map((row) => `| ${row.join(' | ')} |\n`).join('')
}

export function readOrgList(node: OrgNode): OrgListParts | null {
  if (node.type !== 'list') {
    return null
  }

  const items = node.children.map(readOrgListItem)
  if (items.some((item) => item === null)) {
    return null
  }

  const first = items[0]
  if (!first) {
    return null
  }

  if (items.some((item) => item!.ordered !== first.ordered)) {
    return null
  }

  return {
    ordered: first.ordered,
    items: items.map((item) => item!.text),
  }
}

export function emitOrgList(items: string[], ordered: boolean): string {
  return items
    .map((item, index) => `${ordered ? `${index + 1}.` : '-'} ${item}\n`)
    .join('')
}

export function readOrgLink(node: OrgNode): OrgLinkParts | null {
  if (node.type !== 'link') {
    return null
  }

  const pathNode = node.children.find((child) => child.type === 'link.path')
  if (!pathNode) {
    return null
  }

  const url = pathNode.source.replace(/^\[/, '').replace(/\]$/, '')
  const text = node.children
    .filter((child) => child.type === 'text')
    .map((child) => child.source)
    .join('')

  return { url, text: text.length > 0 ? text : url }
}

export function emitOrgLink(url: string, text: string): string {
  return text.length > 0 && text !== url ? `[[${url}][${text}]]` : `[[${url}]]`
}

export function emitOrgParagraph(text: string): string {
  return text.endsWith('\n') ? text : `${text}\n`
}

function collectOrgBridgeNodes(node: OrgNode): OrgNode[] {
  if (node.type === 'document' || node.type === 'section') {
    return node.children.flatMap(collectOrgBridgeNodes)
  }
  return [node]
}

function isModeledOrgBlockName(name: string): boolean {
  return name === 'src' || name === 'example' || name === 'quote'
}

function headingTextFromSource(node: OrgNode): string {
  return trimFinalNewline(node.source).replace(/^\*+\s*/, '')
}

function trimFinalNewline(value: string): string {
  return value.endsWith('\n') ? value.slice(0, -1) : value
}

function readOrgListItem(
  node: OrgNode,
): { ordered: boolean; text: string } | null {
  if (node.type !== 'list.item') {
    return null
  }

  const bullet = node.children.find(
    (child) => child.type === 'list.item.bullet',
  )
  if (!bullet) {
    return null
  }

  return {
    ordered: /^\d+[.)]$/.test(bullet.source),
    text: node.children
      .filter((child) => child.type === 'text')
      .map((child) => child.source)
      .join('')
      .trim(),
  }
}

function orgTextContent(node: OrgNode): string {
  if (node.type === 'text') {
    return node.source
  }
  return node.children.map(orgTextContent).join('')
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
