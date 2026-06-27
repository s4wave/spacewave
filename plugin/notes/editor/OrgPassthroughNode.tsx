import type {
  EditorConfig,
  LexicalEditor,
  SerializedLexicalNode,
  Spread,
} from 'lexical'
import type { JSX } from 'react'

import { DecoratorNode } from 'lexical'

export type SerializedOrgPassthroughNode = Spread<
  { source: string; type: 'org-passthrough'; version: 1 },
  SerializedLexicalNode
>

export class OrgPassthroughNode extends DecoratorNode<JSX.Element> {
  __source: string

  static getType(): string {
    return 'org-passthrough'
  }

  static clone(node: OrgPassthroughNode): OrgPassthroughNode {
    return new OrgPassthroughNode(node.__source, node.__key)
  }

  static importJSON(json: SerializedOrgPassthroughNode): OrgPassthroughNode {
    return new OrgPassthroughNode(json.source)
  }

  constructor(source: string, key?: string) {
    super(key)
    this.__source = source
  }

  exportJSON(): SerializedOrgPassthroughNode {
    return {
      type: 'org-passthrough',
      version: 1,
      source: this.__source,
    }
  }

  createDOM(_config: EditorConfig): HTMLElement {
    if (this.__source.trim().length === 0) {
      const element = document.createElement('div')
      element.className = 'h-3'
      return element
    }

    const element = document.createElement('pre')
    element.className =
      'border-border/60 bg-background-secondary text-muted-foreground my-2 whitespace-pre-wrap rounded border p-2 font-mono text-xs'
    return element
  }

  updateDOM(): false {
    return false
  }

  isInline(): false {
    return false
  }

  getTextContent(): string {
    return this.__source
  }

  getSource(): string {
    return this.__source
  }

  decorate(_editor: LexicalEditor, _config: EditorConfig): JSX.Element {
    if (this.__source.trim().length === 0) {
      return <span aria-hidden="true" />
    }

    return <code>{this.__source}</code>
  }
}

export function $createOrgPassthroughNode(source: string): OrgPassthroughNode {
  return new OrgPassthroughNode(source)
}

export function $isOrgPassthroughNode(
  node: unknown,
): node is OrgPassthroughNode {
  return node instanceof OrgPassthroughNode
}
