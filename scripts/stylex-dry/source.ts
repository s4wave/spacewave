// source holds one parsed file, its stylex.create calls and its pending edits.
import { parseSync } from 'oxc-parser'
import { readFileSync, writeFileSync } from 'node:fs'
import { Exprs } from './values.ts'

/**
 * AstNode is an oxc ESTree node. The passes branch on node.type and read the
 * fields of each shape directly, which a union of every node type would only
 * restate.
 */
// oxlint-disable-next-line typescript/no-explicit-any
export type AstNode = any

/** Edit replaces source[start, end) with text. */
export interface Edit {
  start: number
  end: number
  text: string
}

/** StyleSet is one `const name = stylex.create({...})` declaration. */
export interface StyleSet {
  name: string
  exported: boolean
  /** decl is the VariableDeclaration (or its ExportNamedDeclaration wrapper). */
  stmt: AstNode
  declarator: AstNode
  object: AstNode
}

/** walk visits every AST node depth first. */
export function walk(n: AstNode, f: (n: AstNode) => void) {
  if (!n || typeof n !== 'object') return
  if (typeof n.type === 'string') f(n)
  for (const k in n) {
    const v = n[k]
    if (Array.isArray(v)) for (const x of v) walk(x, f)
    else if (v && typeof v === 'object') walk(v, f)
  }
}

/** isCreate reports whether a node is a create({...}) call on a StyleX namespace import. */
export function isCreate(
  n: AstNode,
  ns: Set<string> = new Set(['stylex']),
): boolean {
  return (
    n?.type === 'CallExpression' &&
    n.callee.type === 'MemberExpression' &&
    n.callee.object.type === 'Identifier' &&
    ns.has(n.callee.object.name) &&
    n.callee.property.name === 'create' &&
    n.arguments[0]?.type === 'ObjectExpression'
  )
}

/** Source is a file under rewrite. */
export class Source {
  readonly src: string
  readonly program: AstNode
  readonly exprs = new Exprs()
  readonly edits: Edit[] = []

  constructor(readonly path: string) {
    this.src = readFileSync(path, 'utf8')
    const parsed = parseSync(path, this.src)
    if (parsed.errors.length)
      throw new Error(`${path}: ${parsed.errors[0].message}`)
    this.program = parsed.program
    for (const stmt of this.program.body) {
      if (
        stmt.type !== 'ImportDeclaration' ||
        stmt.source.value !== '@stylexjs/stylex'
      )
        continue
      for (const sp of stmt.specifiers)
        if (sp.type === 'ImportNamespaceSpecifier')
          this.stylex.add(sp.local.name)
    }
  }

  /** stylex holds the local names bound to the StyleX namespace. */
  readonly stylex = new Set<string>()

  /** text returns a node's source. */
  text(n: AstNode): string {
    return this.src.slice(n.start, n.end)
  }

  /** creates lists every stylex.create object literal in the file. */
  creates(): AstNode[] {
    const out: AstNode[] = []
    walk(this.program, (n) => {
      if (isCreate(n, this.stylex)) out.push(n.arguments[0])
    })
    return out
  }

  /** styleSets lists the module-level `const x = stylex.create(...)` declarations. */
  styleSets(): StyleSet[] {
    const out: StyleSet[] = []
    for (const stmt of this.program.body) {
      const exported = stmt.type === 'ExportNamedDeclaration'
      const decl = exported ? stmt.declaration : stmt
      if (
        decl?.type !== 'VariableDeclaration' ||
        decl.declarations.length !== 1
      )
        continue
      const d = decl.declarations[0]
      if (
        d.id.type !== 'Identifier' ||
        d.id.typeAnnotation ||
        !isCreate(d.init, this.stylex)
      )
        continue
      out.push({
        name: d.id.name,
        exported,
        stmt,
        declarator: d,
        object: d.init.arguments[0],
      })
    }
    return out
  }

  /** importsOf maps local names to imported names for imports whose specifier matches. */
  importsOf(match: (specifier: string) => boolean): Map<string, string> {
    const out = new Map<string, string>()
    for (const stmt of this.program.body) {
      if (stmt.type !== 'ImportDeclaration' || !match(stmt.source.value))
        continue
      for (const s of stmt.specifiers) {
        if (s.type === 'ImportSpecifier')
          out.set(s.local.name, s.imported.name ?? s.imported.value)
      }
    }
    return out
  }

  /** importSpecifier returns the first import specifier string matching a test. */
  importSpecifier(match: (specifier: string) => boolean): string | undefined {
    for (const stmt of this.program.body) {
      if (stmt.type === 'ImportDeclaration' && match(stmt.source.value))
        return stmt.source.value
    }
    return undefined
  }

  /** lastImportEnd returns the offset after the final import declaration. */
  lastImportEnd(): number {
    let end = 0
    for (const stmt of this.program.body)
      if (stmt.type === 'ImportDeclaration') end = stmt.end
    return end
  }

  /** identifiers returns every identifier name used in the file. */
  identifiers(): Set<string> {
    const out = new Set<string>()
    walk(this.program, (n) => {
      if (n.type === 'Identifier') out.add(n.name)
    })
    return out
  }

  /** commit applies the edits and writes the file when anything changed. */
  commit(): boolean {
    if (!this.edits.length) return false
    const edits = [...this.edits].sort(
      (a, b) => b.start - a.start || b.end - a.end,
    )
    let out = this.src
    let floor = Infinity
    for (const e of edits) {
      if (e.end > floor)
        throw new Error(`${this.path}: overlapping edits at ${e.start}`)
      out = out.slice(0, e.start) + e.text + out.slice(e.end)
      floor = e.start
    }
    writeFileSync(this.path, out)
    return true
  }
}

/** keyName returns a property's static key, or undefined for computed keys. */
export function keyName(p: AstNode, src: string): string | undefined {
  if (p.computed) return '[' + src.slice(p.key.start, p.key.end) + ']'
  if (p.key.type === 'Identifier') return p.key.name
  if (p.key.type === 'Literal') return String(p.key.value)
  return undefined
}
