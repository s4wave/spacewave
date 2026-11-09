// kit extracts the shared style groups in web/style/kit from component style sets.
//
// A kit entry is extracted from a style object when every one of its
// properties appears in the object with an identical value and identical
// conditions. Its properties are replaced by a spread of the kit fragment in
// the same entry, so the compiled entry holds the same properties. The build
// inlines the spread before StyleX compiles the file (bldr stylex-fragments).
import { readdirSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { Source, keyName, walk, type StyleSet, type AstNode } from './source.ts'
import { STYLE_DIR, importPath, isTokens } from './passes.ts'

/** KitEntry is one named group of properties in a kit module. */
export interface KitEntry {
  ns: string
  key: string
  props: Map<string, string>
}

/** Kit is the ordered list of kit entries, largest first. */
export type Kit = KitEntry[]

/** Consts holds the static values of the token, space and media modules. */
interface Consts {
  space: Map<string, string>
  media: Map<string, string>
}

/** readConsts parses one defineConsts module into key to string value. */
function readConsts(path: string): Map<string, string> {
  const out = new Map<string, string>()
  const s = new Source(path)
  walk(s.program, (n) => {
    if (
      n.type !== 'CallExpression' ||
      n.callee?.property?.name !== 'defineConsts'
    )
      return
    for (const p of n.arguments[0].properties) {
      const k = keyName(p, s.src)
      if (
        k !== undefined &&
        p.value.type === 'Literal' &&
        typeof p.value.value === 'string'
      )
        out.set(k, p.value.value)
    }
  })
  return out
}

/** Canon renders values of one file in a form comparable across files. */
class Canon {
  private readonly tokens = new Set<string>()
  private readonly space = new Set<string>()
  private readonly media = new Set<string>()
  private readonly locals = new Map<string, AstNode>()

  constructor(
    private readonly s: Source,
    private readonly consts: Consts,
  ) {
    for (const stmt of s.program.body) {
      if (stmt.type === 'ImportDeclaration') {
        const spec: string = stmt.source.value
        for (const sp of stmt.specifiers) {
          if (sp.type !== 'ImportSpecifier') continue
          const imported = sp.imported.name ?? sp.imported.value
          if (isTokens(spec) && imported !== 'keyframeNames')
            this.tokens.add(sp.local.name)
          if (/(^|\/)space\.stylex(\.js)?$/.test(spec) && imported === 'space')
            this.space.add(sp.local.name)
          if (/(^|\/)media\.stylex(\.js)?$/.test(spec) && imported === 'media')
            this.media.add(sp.local.name)
        }
      }
      // Module-level string constants, such as a shared easing, resolve by value.
      const decl =
        stmt.type === 'ExportNamedDeclaration' ? stmt.declaration : stmt
      if (decl?.type === 'VariableDeclaration' && decl.kind === 'const') {
        for (const d of decl.declarations) {
          if (
            d.id.type === 'Identifier' &&
            (d.init?.type === 'Literal' || d.init?.type === 'TemplateLiteral')
          ) {
            this.locals.set(d.id.name, d.init)
          }
        }
      }
    }
  }

  /** member returns the static text of a token, space or media member, if AstNode. */
  private member(n: AstNode): string | undefined {
    if (n.type !== 'MemberExpression' || n.object.type !== 'Identifier')
      return undefined
    const obj = n.object.name
    let key: string | undefined
    if (n.computed && n.property.type === 'Literal')
      key = String(n.property.value)
    else if (!n.computed && n.property.type === 'Identifier')
      key = n.property.name
    if (key === undefined) return undefined
    // defineVars keeps names that begin with --, so a token member is var(--name).
    if (this.tokens.has(obj) && key.startsWith('--')) return `var(${key})`
    if (this.space.has(obj)) return this.consts.space.get(key)
    if (this.media.has(obj)) return this.consts.media.get(key)
    return undefined
  }

  /** text renders an expression used inside a string, or undefined when it is not static. */
  private text(n: AstNode, depth = 0): string | undefined {
    if (depth > 4) return undefined
    if (n.type === 'Literal' && typeof n.value === 'string') return n.value
    if (n.type === 'Literal' && typeof n.value === 'number')
      return this.s.text(n)
    if (n.type === 'TemplateLiteral') {
      let out = ''
      for (let i = 0; i < n.quasis.length; i++) {
        out += n.quasis[i].value.cooked
        if (i < n.expressions.length) {
          const t = this.text(n.expressions[i], depth + 1)
          if (t === undefined) return undefined
          out += t
        }
      }
      return out
    }
    if (n.type === 'Identifier' && this.locals.has(n.name))
      return this.text(this.locals.get(n.name), depth + 1)
    return this.member(n)
  }

  /** key renders a condition key. */
  key(p: AstNode): string | undefined {
    if (!p.computed) return keyName(p, this.s.src)
    const t = this.text(p.key)
    return t === undefined ? undefined : t
  }

  /** value renders a property value, or undefined when it is not static. */
  value(n: AstNode): string | undefined {
    if (n.type === 'Literal' && n.value === null) return 'null'
    if (n.type === 'Literal' && typeof n.value === 'number')
      return 'n:' + this.s.text(n)
    if (
      n.type === 'UnaryExpression' &&
      n.operator === '-' &&
      n.argument.type === 'Literal'
    ) {
      return 'n:' + this.s.text(n)
    }
    if (n.type === 'ObjectExpression') {
      const parts: string[] = []
      for (const p of n.properties) {
        if (p.type !== 'Property') return undefined
        const k = this.key(p)
        const v = k === undefined ? undefined : this.value(p.value)
        if (v === undefined) return undefined
        parts.push(JSON.stringify(k) + ':' + v)
      }
      return '{' + parts.sort().join(',') + '}'
    }
    const t = this.text(n)
    return t === undefined ? undefined : 's:' + t
  }
}

/** loadConsts reads the space and media constants. */
function loadConsts(root: string): Consts {
  return {
    space: readConsts(resolve(root, STYLE_DIR, 'space.stylex.ts')),
    media: readConsts(resolve(root, STYLE_DIR, 'media.stylex.ts')),
  }
}

/** fragmentSets lists the `export const x = {...} as const` objects of a kit module. */
function fragmentSets(s: Source): Array<{ name: string; object: AstNode }> {
  const out: Array<{ name: string; object: AstNode }> = []
  for (const stmt of s.program.body) {
    if (stmt.type !== 'ExportNamedDeclaration') continue
    const decl = stmt.declaration
    if (decl?.type !== 'VariableDeclaration') continue
    for (const d of decl.declarations) {
      let init = d.init
      while (init?.type === 'TSAsExpression' || init?.type === 'TSSatisfiesExpression')
        init = init.expression
      if (d.id.type === 'Identifier' && init?.type === 'ObjectExpression')
        out.push({ name: d.id.name, object: init })
    }
  }
  return out
}

/** loadKit reads every kit fragment module, largest entries first. */
export function loadKit(root: string): { kit: Kit; consts: Consts } {
  const consts = loadConsts(root)
  const dir = resolve(root, STYLE_DIR, 'kit')
  const kit: Kit = []
  for (const f of readdirSync(dir).sort()) {
    if (!f.endsWith('.fragments.ts')) continue
    const s = new Source(resolve(dir, f))
    const canon = new Canon(s, consts)
    for (const set of fragmentSets(s)) {
      for (const p of set.object.properties) {
        const key = keyName(p, s.src)!
        const props = new Map<string, string>()
        for (const q of p.value.properties) {
          const v = canon.value(q.value)
          if (v === undefined)
            throw new Error(
              `${f}: ${set.name}.${key}.${keyName(q, s.src)} is not static`,
            )
          props.set(keyName(q, s.src)!, v)
        }
        kit.push({ ns: set.name, key, props })
      }
    }
  }
  // Largest first, so a three-property group wins over its two-property subset.
  kit.sort((a, b) => b.props.size - a.props.size)
  return { kit, consts }
}

/** kitRef renders a member reference to a kit entry. */
function kitRef(alias: string, e: KitEntry): string {
  return /^[A-Za-z_$][\w$]*$/.test(e.key)
    ? `${alias}.${e.ns}.${e.key}`
    : `${alias}.${e.ns}['${e.key}']`
}

/** afterComma returns the offset after a node's trailing comma and the rest of its line. */
function afterComma(src: string, end: number): number {
  let i = end
  while (i < src.length && /[ \t]/.test(src[i])) i++
  if (src[i] === ',') i++
  while (i < src.length && /[ \t]/.test(src[i])) i++
  if (src[i] === '\n') i++
  return i
}

/** lineStart returns the offset of the first character of the line containing i, if only blanks precede it. */
function lineStart(src: string, i: number): number {
  let j = i
  while (j > 0 && /[ \t]/.test(src[j - 1])) j--
  return j > 0 && src[j - 1] === '\n' ? j : i
}

/** Usage counts how often each kit entry was extracted. */
export type Usage = Map<string, number>

/** kitImport matches the kit index import specifier. */
const kitImport = /style\/kit\.fragments(\.js)?$/

/** extractKit rewrites each module-level style set to spread kit fragments. */
export function extractKit(
  s: Source,
  loaded: { kit: Kit; consts: Consts },
  root: string,
  usage: Usage,
): number {
  const canon = new Canon(s, loaded.consts)
  const alias =
    s.identifiers().has('kit') && !s.importsOf((x) => kitImport.test(x)).size
      ? 'styleKit'
      : 'kit'
  let changed = 0
  for (const set of s.styleSets())
    changed += extractSet(s, set, canon, loaded.kit, alias, usage)
  if (
    changed &&
    ![...s.program.body].some(
      (st: AstNode) =>
        st.type === 'ImportDeclaration' && kitImport.test(st.source.value),
    )
  ) {
    const spec = importPath(
      s.path,
      'kit.fragments.js',
      root,
      s.importSpecifier(isTokens),
    )
    s.edits.push({
      start: s.lastImportEnd(),
      end: s.lastImportEnd(),
      text: `\nimport * as ${alias} from '${spec}'`,
    })
  }
  return changed
}

/**
 * extractSet rewrites one style set in place, returning the number of entries
 * that spread the kit. Each matched kit entry replaces its properties with a
 * spread at the head of the entry; the build inlines the spread before StyleX
 * compiles the set, so the output is the same atomic classes as before.
 */
function extractSet(
  s: Source,
  set: StyleSet,
  canon: Canon,
  kit: Kit,
  alias: string,
  usage: Usage,
): number {
  let changed = 0
  for (const p of set.object.properties) {
    if (p.type !== 'Property' || p.value.type !== 'ObjectExpression') continue
    const props = new Map<
      string,
      { node: AstNode; value: string | undefined }
    >()
    let plain = true
    for (const q of p.value.properties) {
      if (q.type !== 'Property' || q.computed) {
        plain = false
        break
      }
      props.set(keyName(q, s.src)!, { node: q, value: canon.value(q.value) })
    }
    if (!plain) continue
    const taken = new Set<string>()
    const refs: string[] = []
    for (const e of kit) {
      let hit = true
      for (const [k, v] of e.props) {
        const have = props.get(k)
        if (!have || taken.has(k) || have.value !== v) {
          hit = false
          break
        }
      }
      if (!hit) continue
      for (const k of e.props.keys()) taken.add(k)
      refs.push(kitRef(alias, e))
      usage.set(`${e.ns}.${e.key}`, (usage.get(`${e.ns}.${e.key}`) ?? 0) + 1)
    }
    if (!refs.length) continue
    for (const k of taken) {
      const q = props.get(k)!.node
      s.edits.push({
        start: lineStart(s.src, q.start),
        end: afterComma(s.src, q.end),
        text: '',
      })
    }
    const at = p.value.start + 1
    s.edits.push({
      start: at,
      end: at,
      text:
        refs.map((r) => `\n...${r},`).join('') +
        (s.src[at] === '\n' ? '' : '\n'),
    })
    changed++
  }
  return changed
}

/** kitIndex renders web/style/kit.fragments.ts, re-exporting each kit module. */
export function kitIndex(root: string): string {
  const dir = resolve(root, STYLE_DIR, 'kit')
  const names = readdirSync(dir)
    .filter((f) => f.endsWith('.fragments.ts'))
    .sort()
  const lines = names.map((f) => {
    const src = readFileSync(resolve(dir, f), 'utf8')
    const exported = [
      ...src.matchAll(/^export const (\w+) = \{/gm),
    ].map((m) => m[1])
    return `export { ${exported.join(', ')} } from './kit/${f.replace(/\.ts$/, '.js')}'`
  })
  return (
    '// kit re-exports the shared style fragments; spread them into a stylex.create entry.\n' +
    lines.join('\n') +
    '\n'
  )
}
