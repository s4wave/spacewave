import type {
  EditorConfig,
  LexicalEditor,
  SerializedLexicalNode,
  Spread,
} from 'lexical'
import type { TextMatchTransformer } from '@lexical/markdown'
import type { JSX } from 'react'

import { DecoratorNode } from 'lexical'
import { LuLayers } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { marginBlock: `calc(${themeLayout['--spacing']} * 2)` },
})

const stylexStyles2 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: themeColors['--color-border'],
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-card'],
    display: 'flex',
  },
  rule2: {
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
  },
  rule3: { flex: 1, minWidth: 0, },
  rule4: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule6: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

type SerializedSpacewaveEmbedNode = Spread<
  { path: string; type: 'spacewave-embed'; version: 1 },
  SerializedLexicalNode
>

// SpacewaveEmbedNode renders a ::spacewave{path="..."} directive as a visual block.
export class SpacewaveEmbedNode extends DecoratorNode<JSX.Element> {
  __path: string

  static getType(): string {
    return 'spacewave-embed'
  }

  static clone(node: SpacewaveEmbedNode): SpacewaveEmbedNode {
    return new SpacewaveEmbedNode(node.__path, node.__key)
  }

  static importJSON(json: SerializedSpacewaveEmbedNode): SpacewaveEmbedNode {
    return new SpacewaveEmbedNode(json.path)
  }

  constructor(path: string, key?: string) {
    super(key)
    this.__path = path
  }

  exportJSON(): SerializedSpacewaveEmbedNode {
    return {
      ...super.exportJSON(),
      path: this.__path,
      type: 'spacewave-embed',
      version: 1,
    }
  }

  createDOM(_config: EditorConfig): HTMLElement {
    const div = document.createElement('div')
    div.className = stylex.props(stylexStyles3.rule1).className ?? ''
    return div
  }

  updateDOM(): false {
    return false
  }

  isInline(): false {
    return false
  }

  getTextContent(): string {
    return `::spacewave{path="${this.__path}"}`
  }

  decorate(_editor: LexicalEditor, _config: EditorConfig): JSX.Element {
    const filename = this.__path.split('/').pop() ?? this.__path
    const ext = filename.includes('.') ? (filename.split('.').pop() ?? '') : ''

    return (
      <div sx={stylexStyles2.rule1}>
        <div sx={stylexStyles2.rule2}>
          <LuLayers sx={stylexStyles.rule1} />
        </div>
        <div sx={stylexStyles2.rule3}>
          <div sx={stylexStyles2.rule4}>{this.__path}</div>
          <div sx={stylexStyles2.rule5}>
            <span sx={stylexStyles2.rule6}>spacewave</span>
            {ext && <span sx={stylexStyles2.rule7}>{ext}</span>}
          </div>
        </div>
      </div>
    )
  }
}

// SPACEWAVE_EMBED_TRANSFORMER handles round-tripping ::spacewave{path="..."} directives.
export const SPACEWAVE_EMBED_TRANSFORMER: TextMatchTransformer = {
  dependencies: [SpacewaveEmbedNode],
  export: (node) => {
    if (node instanceof SpacewaveEmbedNode) {
      return `::spacewave{path="${node.__path}"}`
    }
    return null
  },
  importRegExp: /::spacewave\{path="([^"]+)"\}/,
  regExp: /::spacewave\{path="([^"]+)"\}/,
  replace: (textNode, match) => {
    const node = new SpacewaveEmbedNode(match[1])
    textNode.replace(node)
  },
  type: 'text-match',
}
