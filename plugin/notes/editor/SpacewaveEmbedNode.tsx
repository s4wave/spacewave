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
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-border'],
    backgroundColor: themeColors['--color-card'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    color: themeColors['--color-brand'],
  },
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    borderRadius: '.25rem',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
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
