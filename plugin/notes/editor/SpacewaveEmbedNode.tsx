import type {
  EditorConfig,
  LexicalEditor,
  SerializedLexicalNode,
  Spread,
} from 'lexical'
import type { TextMatchTransformer } from '@lexical/markdown'
import type { JSX } from 'react'

import { DecoratorNode } from 'lexical'
import { LuLayers } from 'react-icons/lu'

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
    div.className = 'my-2'
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
    const ext = filename.includes('.') ? filename.split('.').pop() ?? '' : ''

    return (
      <div className="bg-card border-border flex items-center gap-3 rounded-lg border p-3">
        <div className="text-brand flex items-center">
          <LuLayers className="h-5 w-5" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-foreground truncate text-xs font-medium">
            {this.__path}
          </div>
          <div className="mt-0.5 flex items-center gap-2">
            <span className="bg-brand/10 text-brand rounded px-1.5 py-0.5 text-xs font-medium">
              spacewave
            </span>
            {ext && (
              <span className="text-muted-foreground text-xs">{ext}</span>
            )}
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
