import { $createCodeNode, $isCodeNode } from '@lexical/code'
import { $createLinkNode, $isLinkNode } from '@lexical/link'
import {
  $createListItemNode,
  $createListNode,
  $isListItemNode,
  $isListNode,
} from '@lexical/list'
import {
  $createHeadingNode,
  $createQuoteNode,
  $isHeadingNode,
  $isQuoteNode,
  type HeadingTagType,
} from '@lexical/rich-text'
import {
  $createTableCellNode,
  $createTableNode,
  $createTableRowNode,
  $isTableCellNode,
  $isTableNode,
  $isTableRowNode,
} from '@lexical/table'
import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $getState,
  $isElementNode,
  $isParagraphNode,
  $isTextNode,
  $setState,
  createState,
  type ElementNode,
  type LexicalNode,
} from 'lexical'

import {
  $createOrgPassthroughNode,
  $isOrgPassthroughNode,
} from '../editor/OrgPassthroughNode.js'
import {
  emitOrgBlockText,
  emitOrgHeadingText,
  emitOrgLink,
  emitOrgList,
  emitOrgParagraph,
  emitOrgTable,
  getOrgBridgeSegments,
  parseOrg,
  readOrgBlock,
  readOrgHeading,
  readOrgLink,
  readOrgList,
  readOrgTable,
  type OrgBridgeSegment,
  type OrgNode,
} from './org.js'

// orgSourceState holds the Org text a top-level node was imported from.
const orgSourceState = createState('orgSource', {
  parse: (value) => (typeof value === 'string' ? value : ''),
})

// orgExportedState holds what the node exported to when it was imported. A node
// that still exports to this text is untouched, so its source is emitted
// instead of the normalized export.
const orgExportedState = createState('orgExported', {
  parse: (value) => (typeof value === 'string' ? value : ''),
})

export function $convertFromOrgString(org: string, node?: ElementNode): void {
  const target = node ?? $getRoot()
  target.clear()

  const document = parseOrg(org)
  const segments = getOrgBridgeSegments(document)
  if (segments.length === 0) {
    target.append($createParagraphNode())
    return
  }

  for (const segment of segments) {
    appendOrgSegment(target, segment)
  }
}

export function $convertToOrgString(node?: ElementNode): string {
  const source = node ?? $getRoot()
  return source.getChildren().map(exportOrgNode).join('')
}

function appendOrgSegment(
  parent: ElementNode,
  segment: OrgBridgeSegment,
): void {
  const node = createOrgSegmentNode(segment)
  if (!$isOrgPassthroughNode(node)) {
    $setState(node, orgSourceState, segment.source)
    $setState(node, orgExportedState, emitOrgNode(node))
  }
  parent.append(node)
}

function createOrgSegmentNode(segment: OrgBridgeSegment): LexicalNode {
  if (segment.kind === 'passthrough') {
    return $createOrgPassthroughNode(segment.source)
  }

  switch (segment.modeledKind) {
    case 'headline': {
      const heading = readOrgHeading(segment.node)
      if (!heading) {
        return $createOrgPassthroughNode(segment.source)
      }
      const headingNode = $createHeadingNode(headingTagFromLevel(heading.level))
      headingNode.append($createTextNode(heading.text))
      return headingNode
    }
    case 'paragraph': {
      const paragraph = $createParagraphNode()
      appendOrgInlineChildren(paragraph, segment.node.children)
      return paragraph
    }
    case 'block':
      return createOrgBlockNode(segment)
    case 'table':
      return createOrgTableNode(segment)
    case 'list':
      return createOrgListNode(segment)
  }
}

function createOrgBlockNode(segment: OrgBridgeSegment): LexicalNode {
  if (segment.kind !== 'modeled') {
    return $createOrgPassthroughNode(segment.source)
  }

  const block = readOrgBlock(segment.node)
  if (!block) {
    return $createOrgPassthroughNode(segment.source)
  }

  if (block.name === 'quote') {
    const quote = $createQuoteNode()
    if (block.value.length > 0) {
      quote.append($createTextNode(block.value))
    }
    return quote
  }

  const code = $createCodeNode(
    block.name === 'src' ? (block.params[0] ?? null) : null,
  )
  if (block.value.length > 0) {
    code.append($createTextNode(block.value))
  }
  return code
}

function createOrgTableNode(segment: OrgBridgeSegment): LexicalNode {
  if (segment.kind !== 'modeled') {
    return $createOrgPassthroughNode(segment.source)
  }

  const table = readOrgTable(segment.node)
  if (!table) {
    return $createOrgPassthroughNode(segment.source)
  }

  const tableNode = $createTableNode()
  for (const row of table.rows) {
    const rowNode = $createTableRowNode()
    for (const cell of row) {
      const cellNode = $createTableCellNode()
      const paragraph = $createParagraphNode()
      if (cell.length > 0) {
        paragraph.append($createTextNode(cell))
      }
      cellNode.append(paragraph)
      rowNode.append(cellNode)
    }
    tableNode.append(rowNode)
  }
  return tableNode
}

function createOrgListNode(segment: OrgBridgeSegment): LexicalNode {
  if (segment.kind !== 'modeled') {
    return $createOrgPassthroughNode(segment.source)
  }

  const list = readOrgList(segment.node)
  if (!list) {
    return $createOrgPassthroughNode(segment.source)
  }

  const listNode = $createListNode(list.ordered ? 'number' : 'bullet')
  for (const item of list.items) {
    const itemNode = $createListItemNode()
    if (item.length > 0) {
      itemNode.append($createTextNode(item))
    }
    listNode.append(itemNode)
  }
  return listNode
}

function appendOrgInlineChildren(
  parent: ElementNode,
  children: OrgNode[],
): void {
  for (const child of children) {
    if (child.type === 'newline' || child.type === 'emptyLine') {
      continue
    }

    if (child.type === 'link') {
      const link = readOrgLink(child)
      if (link) {
        const linkNode = $createLinkNode(link.url)
        linkNode.append($createTextNode(link.text))
        parent.append(linkNode)
        continue
      }
    }

    if (child.source.length > 0) {
      parent.append($createTextNode(child.source))
    }
  }
}

function exportOrgNode(node: LexicalNode): string {
  const text = emitOrgNode(node)
  return text === $getState(node, orgExportedState)
    ? $getState(node, orgSourceState)
    : text
}

function emitOrgNode(node: LexicalNode): string {
  if ($isOrgPassthroughNode(node)) {
    return node.getSource()
  }

  if ($isHeadingNode(node)) {
    return emitOrgHeadingText(
      levelFromHeadingTag(node.getTag()),
      exportInlineChildren(node),
    )
  }

  if ($isParagraphNode(node)) {
    return emitOrgParagraph(exportInlineChildren(node))
  }

  if ($isCodeNode(node)) {
    const language = node.getLanguage()
    return emitOrgBlockText(
      language ? 'src' : 'example',
      language ? [language] : [],
      node.getTextContent(),
    )
  }

  if ($isQuoteNode(node)) {
    return emitOrgBlockText('quote', [], node.getTextContent())
  }

  if ($isTableNode(node)) {
    return emitOrgTable(exportTableRows(node))
  }

  if ($isListNode(node)) {
    return emitOrgList(
      node
        .getChildren()
        .flatMap((item) =>
          $isListItemNode(item) ? [item.getTextContent()] : [],
        ),
      node.getListType() === 'number',
    )
  }

  return emitOrgParagraph(node.getTextContent())
}

function exportInlineChildren(parent: ElementNode): string {
  return parent.getChildren().map(exportInlineNode).join('')
}

function exportInlineNode(node: LexicalNode): string {
  if ($isTextNode(node)) {
    return node.getTextContent()
  }

  if ($isLinkNode(node)) {
    return emitOrgLink(node.getURL(), exportInlineChildren(node))
  }

  if ($isElementNode(node)) {
    return exportInlineChildren(node)
  }

  return node.getTextContent()
}

function exportTableRows(table: ElementNode): string[][] {
  return table
    .getChildren()
    .flatMap((row) =>
      $isTableRowNode(row)
        ? [
            row
              .getChildren()
              .flatMap((cell) =>
                $isTableCellNode(cell) ? [cell.getTextContent().trim()] : [],
              ),
          ]
        : [],
    )
}

function headingTagFromLevel(level: number): HeadingTagType {
  return `h${Math.max(1, Math.min(6, level))}` as HeadingTagType
}

function levelFromHeadingTag(tag: HeadingTagType): number {
  return Number(tag.slice(1))
}
