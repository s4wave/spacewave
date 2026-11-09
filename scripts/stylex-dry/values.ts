// values models StyleX property values as text with opaque expression slots.
//
// A leaf value is rendered to a "parts string": literal CSS text in which each
// embedded JavaScript expression is replaced by a placeholder. The passes edit
// that text and the emitter turns it back into a string or template literal.

import type { AstNode } from './source.ts'

/** OPEN and CLOSE delimit an expression placeholder inside a parts string. */
const OPEN = ''
const CLOSE = ''

/** Exprs interns the expression sources referenced by one file's parts strings. */
export class Exprs {
  private readonly sources: string[] = []
  private readonly index = new Map<string, number>()

  /** slot returns the placeholder text for an expression source. */
  slot(source: string): string {
    let i = this.index.get(source)
    if (i === undefined) {
      i = this.sources.length
      this.sources.push(source)
      this.index.set(source, i)
    }
    return OPEN + i + CLOSE
  }

  /** source returns the expression source behind a placeholder index. */
  source(i: number): string {
    return this.sources[i]
  }

  /** split breaks a parts string into literal text and expression sources. */
  split(parts: string): Array<{ text: string } | { expr: string }> {
    const out: Array<{ text: string } | { expr: string }> = []
    let at = 0
    for (;;) {
      const open = parts.indexOf(OPEN, at)
      if (open < 0) break
      const close = parts.indexOf(CLOSE, open)
      if (open > at) out.push({ text: parts.slice(at, open) })
      out.push({ expr: this.source(Number(parts.slice(open + 1, close))) })
      at = close + 1
    }
    if (at < parts.length) out.push({ text: parts.slice(at) })
    return out
  }

  /** emit renders a parts string as the shortest equivalent JavaScript expression. */
  emit(parts: string): string {
    const pieces = this.split(parts)
    if (pieces.length === 1 && 'expr' in pieces[0]) return pieces[0].expr
    if (pieces.every((p) => 'text' in p)) {
      const text = pieces.map((p) => (p as { text: string }).text).join('')
      return "'" + text.replace(/\\/g, '\\\\').replace(/'/g, "\\'") + "'"
    }
    return (
      '`' +
      pieces
        .map((p) =>
          'text' in p
            ? p.text
                .replace(/\\/g, '\\\\')
                .replace(/`/g, '\\`')
                .replace(/\$\{/g, '\\${')
            : '${' + p.expr + '}',
        )
        .join('') +
      '`'
    )
  }
}

/** Leaf is a property value at one condition path. */
export type Leaf =
  | { kind: 'null' }
  | { kind: 'number'; text: string }
  | { kind: 'text'; parts: string }
  | { kind: 'opaque'; source: string }

/** readLeaf converts a value node into a Leaf, interning expressions in exprs. */
export function readLeaf(node: AstNode, src: string, exprs: Exprs): Leaf {
  if (node.type === 'Literal') {
    if (node.value === null) return { kind: 'null' }
    if (typeof node.value === 'number')
      return { kind: 'number', text: src.slice(node.start, node.end) }
    if (typeof node.value === 'string')
      return { kind: 'text', parts: node.value }
  }
  if (node.type === 'TemplateLiteral') {
    let parts = ''
    node.quasis.forEach((q: AstNode, i: number) => {
      parts += q.value.cooked
      if (i < node.expressions.length) {
        const e = node.expressions[i]
        parts += exprs.slot(src.slice(e.start, e.end))
      }
    })
    return { kind: 'text', parts }
  }
  if (
    node.type === 'UnaryExpression' &&
    node.operator === '-' &&
    node.argument.type === 'Literal'
  ) {
    return { kind: 'number', text: src.slice(node.start, node.end) }
  }
  if (node.type === 'MemberExpression' || node.type === 'Identifier') {
    return { kind: 'text', parts: exprs.slot(src.slice(node.start, node.end)) }
  }
  return { kind: 'opaque', source: src.slice(node.start, node.end) }
}

/** leafText renders a leaf as CSS text for substitution, or undefined when it has none. */
export function leafText(leaf: Leaf): string | undefined {
  if (leaf.kind === 'text') return leaf.parts
  if (leaf.kind === 'number') return leaf.text
  return undefined
}

/** VarRef is one var() reference found in a parts string. */
export interface VarRef {
  start: number
  end: number
  name: string
  fallback?: string
}

/** findVars returns the outermost var() references in text, in order. */
export function findVars(text: string): VarRef[] {
  const out: VarRef[] = []
  let at = 0
  for (;;) {
    const start = text.indexOf('var(', at)
    if (start < 0) return out
    // Only a var( that is not the tail of another identifier.
    if (start > 0 && /[\w-]/.test(text[start - 1])) {
      at = start + 4
      continue
    }
    let depth = 0
    let comma = -1
    let end = -1
    for (let i = start + 3; i < text.length; i++) {
      const c = text[i]
      if (c === '(') depth++
      else if (c === ')') {
        depth--
        if (depth === 0) {
          end = i + 1
          break
        }
      } else if (c === ',' && depth === 1 && comma < 0) comma = i
    }
    if (end < 0) return out
    const inner = text.slice(start + 4, end - 1)
    const ref: VarRef = {
      start,
      end,
      name: (comma < 0 ? inner : text.slice(start + 4, comma)).trim(),
    }
    if (comma >= 0)
      ref.fallback = text.slice(comma + 1, end - 1).replace(/^ /, '')
    out.push(ref)
    at = end
  }
}

/** Cond is a property value flattened to its condition paths. */
export type Cond = Map<string, { leaf: Leaf; node: AstNode }>

/** condKey canonicalizes a condition key node. */
export type KeyReader = (prop: AstNode) => string | undefined

/** flatten maps each condition path of a value node to its leaf. */
export function flatten(
  node: AstNode,
  src: string,
  exprs: Exprs,
  readKey: KeyReader,
  path = '',
): Cond | undefined {
  const out: Cond = new Map()
  if (node.type !== 'ObjectExpression') {
    out.set(path, { leaf: readLeaf(node, src, exprs), node })
    return out
  }
  for (const p of node.properties) {
    if (p.type !== 'Property') return undefined
    const k = readKey(p)
    if (k === undefined) return undefined
    const sub = flatten(
      p.value,
      src,
      exprs,
      readKey,
      k === 'default' ? path : path + '\u0000' + k,
    )
    if (!sub) return undefined
    for (const [kk, v] of sub) out.set(kk, v)
  }
  return out
}

/** effective returns the leaf that applies at a condition path, honoring null as no declaration. */
export function effective(cond: Cond, path: string): Leaf | undefined {
  let p = path
  for (;;) {
    const hit = cond.get(p)
    if (hit && hit.leaf.kind !== 'null') return hit.leaf
    if (p === '') return undefined
    p = p.slice(0, p.lastIndexOf('\u0000'))
  }
}
