import type {
  EditorConfig,
  LexicalEditor,
  SerializedLexicalNode,
  Spread,
} from 'lexical'
import type { JSX } from 'react'

import { DecoratorNode } from 'lexical'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { height: `calc(${themeLayout['--spacing']} * 3)` },
  rule2: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-border']} 60%, transparent)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    marginBlock: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background-secondary'],
    color: themeColors['--color-muted-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    whiteSpace: 'pre-wrap',
  },
})

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
      element.className = stylex.props(stylexStyles.rule1).className ?? ''
      return element
    }

    const element = document.createElement('pre')
    element.className = stylex.props(stylexStyles.rule2).className ?? ''
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
