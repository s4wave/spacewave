// passes implements the value rewrites that keep each element's computed style.
import { relative, dirname } from 'node:path'
import { Source, keyName, walk, type AstNode } from './source.ts'
import {
  type Cond,
  type VarRef,
  effective,
  findVars,
  flatten,
  leafText,
} from './values.ts'

/** STYLE_DIR is the shared style directory, relative to the repository root. */
export const STYLE_DIR = 'web/style'

/** isTokens matches an import of the theme tokens module. */
export const isTokens = (s: string) => /(^|\/)tokens\.stylex(\.js)?$/.test(s)

/** importPath returns the specifier a file uses for a module in web/style. */
export function importPath(
  file: string,
  module: string,
  root: string,
  tokenSpec?: string,
): string {
  if (tokenSpec?.startsWith('@s4wave/web/'))
    return '@s4wave/web/style/' + module
  let rel = relative(dirname(file), root + '/' + STYLE_DIR + '/' + module)
  if (!rel.startsWith('.')) rel = './' + rel
  return rel
}

/** Prop is one property of a style object with its flattened conditions. */
interface Prop {
  node: AstNode
  name: string
  cond?: Cond
}

/** styleObjects yields every style object (a value of a stylex.create key). */
export function* styleObjects(s: Source): Generator<AstNode> {
  for (const create of s.creates()) {
    for (const p of create.properties) {
      if (p.type === 'Property' && p.value.type === 'ObjectExpression')
        yield p.value
    }
  }
}

/** readProps flattens a style object's properties. */
function readProps(s: Source, obj: AstNode): Map<string, Prop> | undefined {
  const out = new Map<string, Prop>()
  for (const p of obj.properties) {
    if (p.type !== 'Property') return undefined
    const name = keyName(p, s.src)
    if (name === undefined) return undefined
    out.set(name, {
      node: p,
      name,
      cond: flatten(p.value, s.src, s.exprs, (q) => keyName(q, s.src)),
    })
  }
  return out
}

/** UNSET marks a setter that clears a variable with null somewhere. */
const UNSET = '\u0000unset'

/** TwValues maps each --tw-* variable to every value AstNode setter in the codebase gives it. */
export type TwValues = Map<string, Set<string>>

/** canon renders a parts string with its expressions inlined, for comparison across files. */
function canon(s: Source, parts: string): string {
  return s.exprs
    .split(parts)
    .map((p) => ('text' in p ? p.text : '${' + p.expr + '}'))
    .join('')
}

/** collectTwValues records the values every style object and CSS rule assigns to --tw-* variables. */
export function collectTwValues(s: Source, into: TwValues) {
  for (const obj of styleObjects(s)) {
    const props = readProps(s, obj)
    for (const p of obj.properties) {
      if (p.type !== 'Property') continue
      const name = keyName(p, s.src)
      if (!name?.startsWith('--tw-')) continue
      const set = into.get(name) ?? new Set<string>()
      into.set(name, set)
      const cond = props?.get(name)?.cond
      if (!cond) {
        // An unreadable setter can hold anything; no reader of it is resolvable.
        set.add(UNSET + 'opaque')
        continue
      }
      for (const { leaf } of cond.values()) {
        const text = leafText(leaf)
        if (leaf.kind === 'null') set.add(UNSET)
        else set.add(text === undefined ? UNSET + 'opaque' : canon(s, text))
      }
    }
  }
}

/** collectCssTwValues records --tw-* assignments made by CSS declarations. */
export function collectCssTwValues(
  css: string,
  initial: Map<string, string>,
  into: TwValues,
) {
  for (const m of css.matchAll(/(--tw-[\w-]+)\s*:\s*([^;{}]*)[;}]/g)) {
    const set = into.get(m[1]) ?? new Set<string>()
    into.set(m[1], set)
    const v = m[2].replace(/\s*!important\s*$/, '').trim()
    // Tailwind's reset for engines without @property assigns initial, or the
    // registered initial value; both equal the unset value.
    set.add(v === 'initial' || v === initial.get(m[1]) ? UNSET : v)
  }
}

/** resolveTw substitutes --tw-* variables whose value is the same wherever the variable can come from. */
export function resolveTw(
  s: Source,
  initial: Map<string, string>,
  values: TwValues,
): number {
  let changed = 0
  for (const obj of styleObjects(s)) {
    const props = readProps(s, obj)
    if (!props) continue
    for (const prop of props.values()) {
      if (!prop.cond) continue
      for (const [path, { leaf, node }] of prop.cond) {
        const text = leafText(leaf)
        if (leaf.kind !== 'text' || !text?.includes('var(--tw-')) continue
        const resolved = resolveText(s, text, path, props, initial, values, 0)
        const next = resolved === undefined ? undefined : tidy(resolved)
        if (next === undefined || next === text) continue
        s.edits.push({
          start: node.start,
          end: node.end,
          text: s.exprs.emit(next),
        })
        changed++
      }
    }
  }
  return changed
}

/**
 * resolveText rewrites the --tw-* references in one leaf, or returns undefined when it cannot.
 *
 * Any style object applied to the same element can set a --tw-* variable, so a
 * reference resolves only when every candidate agrees: each setter in the
 * codebase, the object's own setter at this condition, and the fallback or
 * registered initial value that applies when nothing sets it.
 */
function resolveText(
  s: Source,
  text: string,
  path: string,
  props: Map<string, Prop>,
  initial: Map<string, string>,
  values: TwValues,
  depth: number,
): string | undefined {
  if (depth > 8) return undefined
  let out = ''
  let at = 0
  for (const ref of findVars(text)) {
    out += text.slice(at, ref.start)
    at = ref.end
    if (!ref.name.startsWith('--tw-')) {
      out += text.slice(ref.start, ref.end)
      continue
    }
    const value = resolveRef(s, ref, path, props, initial, values)
    const resolved =
      value === undefined
        ? undefined
        : resolveText(s, value, path, props, initial, values, depth + 1)
    // An unresolvable reference stays as written; its neighbors may still resolve.
    out += resolved ?? text.slice(ref.start, ref.end)
  }
  return out + text.slice(at)
}

/** resolveRef returns the single value a --tw-* reference can take, or undefined. */
function resolveRef(
  s: Source,
  ref: VarRef,
  path: string,
  props: Map<string, Prop>,
  initial: Map<string, string>,
  values: TwValues,
): string | undefined {
  const set = values.get(ref.name) ?? new Set<string>()
  if ([...set].some((v) => v.startsWith(UNSET + 'opaque'))) return undefined
  let own: string | undefined
  const setter = props.get(ref.name)
  if (setter) {
    if (!setter.cond) return undefined
    const eff = effective(setter.cond, path)
    if (eff) {
      own = leafText(eff)
      if (own === undefined) return undefined
    }
  }
  // A registered initial value replaces the guaranteed-invalid value, so var() never reaches its fallback.
  const unset = initial.has(ref.name) ? initial.get(ref.name) : ref.fallback
  // The unset value applies whenever no setter is active, which only an own setter rules out.
  const needUnset = own === undefined || set.has(UNSET)
  if (needUnset && unset === undefined) return undefined
  const candidates = new Set([...set].filter((v) => v !== UNSET))
  if (own !== undefined) candidates.add(canon(s, own))
  if (needUnset) candidates.add(canon(s, unset!))
  if (candidates.size !== 1) return undefined
  return own ?? unset!
}

/** tidy normalizes whitespace and zero offsets left behind by substitution. */
export function tidy(text: string): string {
  if (/['"]/.test(text)) return text
  return text
    .replace(/ {2,}/g, ' ')
    .trim()
    .replace(/calc\((-?[\d.]+px) \+ 0px\)/g, '$1')
}

/** registeredInitials reads @property initial values from CSS. */
export function registeredInitials(css: string): Map<string, string> {
  const out = new Map<string, string>()
  const re = /@property\s+(--[\w-]+)\s*\{([^}]*)\}/g
  for (let m; (m = re.exec(css));) {
    const iv = /initial-value:\s*([^;]*);/.exec(m[2])
    // A registered property without an initial value resolves to the guaranteed-invalid value.
    if (iv) out.set(m[1], iv[1].trim())
  }
  return out
}

/** removeProps deletes every --tw-* setter whose variable nothing reads. */
export function removeDeadSetters(s: Source, read: Set<string>): number {
  let n = 0
  for (const obj of styleObjects(s)) {
    for (const p of obj.properties) {
      if (p.type !== 'Property') continue
      const name = keyName(p, s.src)
      if (!name?.startsWith('--tw-') || read.has(name)) continue
      s.edits.push({ start: p.start, end: afterComma(s.src, p.end), text: '' })
      n++
    }
  }
  return n
}

/** afterComma returns the offset after a property's trailing comma, if AstNode. */
function afterComma(src: string, end: number): number {
  let i = end
  while (i < src.length && /\s/.test(src[i])) i++
  return src[i] === ',' ? i + 1 : end
}

/** readVars collects every --tw-* variable a text reads, by var() or by a token member such as utilityState['--tw-border-style']. */
export function readVars(text: string, into: Set<string>) {
  for (const m of text.matchAll(/var\(\s*(--tw-[\w-]+)/g)) into.add(m[1])
  for (const m of text.matchAll(/\[\s*['"](--tw-[\w-]+)['"]\s*\]/g))
    into.add(m[1])
}

/** SpaceScale collects the spacing steps used across files. */
export type SpaceScale = Set<number>

/** spacing rewrites calc(var(--spacing) * n) into the space scale. */
export function spacing(s: Source, scale: SpaceScale, root: string): number {
  const aliases = s.importsOf(isTokens)
  const layoutAliases = [...aliases]
    .filter(([, imported]) => imported === 'layout')
    .map(([local]) => local)
  const ids = s.identifiers()
  const spaceName =
    ids.has('space') &&
    !s.importsOf((x) => /space\.stylex/.test(x)).has('space')
      ? 'spaceScale'
      : 'space'
  const sp = layoutAliases.map((a) => s.exprs.slot(`${a}['--spacing']`))
  let changed = 0
  for (const obj of styleObjects(s)) {
    walk(obj, (n) => {
      if (n.type !== 'Literal' && n.type !== 'TemplateLiteral') return
      if (n.type === 'Literal' && typeof n.value !== 'string') return
      // Keys are literals too; only rewrite values.
      const text = n.type === 'Literal' ? n.value : leafText(flattenLeaf(s, n))
      if (typeof text !== 'string') return
      const next = rewriteSpacing(text, sp, (k) => {
        scale.add(k)
        return s.exprs.slot(spaceRef(spaceName, k))
      })
      if (next === text) return
      s.edits.push({ start: n.start, end: n.end, text: s.exprs.emit(next) })
      changed++
    })
    // A bare spacing token as a whole value becomes the first step.
    if (layoutAliases.length) {
      walk(obj, (n) => {
        if (
          n.type === 'MemberExpression' &&
          layoutAliases.some((a) => s.text(n) === `${a}['--spacing']`) &&
          !insideTemplate(obj, n)
        ) {
          scale.add(1)
          s.edits.push({ start: n.start, end: n.end, text: `${spaceName}[1]` })
          changed++
        }
      })
    }
  }
  if (changed) {
    const spec = importPath(
      s.path,
      'space.stylex.js',
      root,
      s.importSpecifier(isTokens),
    )
    const name = spaceName === 'space' ? 'space' : 'space as spaceScale'
    if (!s.importsOf((x) => /space\.stylex/.test(x)).size) {
      s.edits.push({
        start: s.lastImportEnd(),
        end: s.lastImportEnd(),
        text: `\nimport { ${name} } from '${spec}'`,
      })
    }
  }
  return changed
}

/** flattenLeaf reads one literal node as a leaf. */
function flattenLeaf(s: Source, n: AstNode) {
  return flatten(n, s.src, s.exprs, () => undefined)!.get('')!.leaf
}

/** insideTemplate reports whether node n sits inside a template literal within root. */
function insideTemplate(root: AstNode, target: AstNode): boolean {
  let found = false
  walk(root, (n) => {
    if (
      n.type === 'TemplateLiteral' &&
      n.start <= target.start &&
      target.end <= n.end
    )
      found = true
  })
  return found
}

/** spaceRef renders a reference to one step of the scale, quoting keys that are not plain integers. */
export function spaceRef(name: string, k: number): string {
  return Number.isInteger(k) && k >= 0 ? `${name}[${k}]` : `${name}['${k}']`
}

/** NUM matches a CSS number. */
const NUM = '(-?\\d*\\.?\\d+)'

/** rewriteSpacing replaces spacing arithmetic in one parts string. */
export function rewriteSpacing(
  text: string,
  sp: string[],
  step: (k: number) => string,
): string {
  const SP = '(?:' + ['var\\(--spacing\\)', ...sp.map(escape)].join('|') + ')'
  let t = text
  // Tailwind's stack reversal terms reduce to constants once the variable is resolved.
  t = t.replace(/calc\(1 - 0\)/g, '1')
  t = t.replace(new RegExp(`calc\\(${SP} \\* ${NUM} \\* 0\\)`, 'g'), '0')
  t = t.replace(
    new RegExp(`calc\\(${SP} \\* ${NUM}(?: \\* 1)?\\)`, 'g'),
    (_, k) => step(Number(k)),
  )
  t = t.replace(new RegExp(`calc\\(${NUM} \\* ${SP}\\)`, 'g'), (_, k) =>
    step(Number(k)),
  )
  t = t.replace(new RegExp(`${SP} \\* ${NUM}(?![\\d.])`, 'g'), (_, k) =>
    step(Number(k)),
  )
  t = t.replace(new RegExp(SP, 'g'), () => step(1))
  return t
}

/** escape quotes a string for a regular expression. */
function escape(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/** spaceModule renders web/style/space.stylex.ts for the steps in use. */
export function spaceModule(scale: SpaceScale): string {
  const steps = [...scale].sort((a, b) => a - b)
  const value = (k: number) => {
    if (k === 0) return '0'
    return `${Number((k * 0.25).toFixed(4))}rem`
  }
  const key = (k: number) =>
    Number.isInteger(k) && k >= 0 ? String(k) : `'${k}'`
  return (
    `import * as stylex from '@stylexjs/stylex'\n\n` +
    `/** space is the spacing scale: step n is n quarter rems, matching the theme's --spacing. */\n` +
    `export const space = stylex.defineConsts({\n` +
    steps.map((k) => `  ${key(k)}: '${value(k)}',\n`).join('') +
    `})\n`
  )
}

/** pruneImports drops named imports from StyleX token modules that the file no longer references. */
export function pruneImports(s: Source): number {
  const used = new Set<string>()
  for (const stmt of s.program.body) {
    if (stmt.type === 'ImportDeclaration') continue
    walk(stmt, (n) => {
      if (n.type === 'Identifier') used.add(n.name)
    })
  }
  let n = 0
  for (const stmt of s.program.body) {
    // A StyleX namespace import left without uses goes too.
    if (
      stmt.type === 'ImportDeclaration' &&
      stmt.source.value === '@stylexjs/stylex'
    ) {
      const [ns, ...rest] = stmt.specifiers
      if (
        ns?.type === 'ImportNamespaceSpecifier' &&
        !rest.length &&
        stmt.importKind !== 'type' &&
        !used.has(ns.local.name)
      ) {
        s.edits.push({
          start: stmt.start,
          end: lineEnd(s.src, stmt.end),
          text: '',
        })
        n++
      }
      continue
    }
    if (
      stmt.type !== 'ImportDeclaration' ||
      !/\.stylex(\.js)?$/.test(stmt.source.value)
    )
      continue
    const dead = stmt.specifiers.filter(
      (sp: AstNode) =>
        sp.type === 'ImportSpecifier' && !used.has(sp.local.name),
    )
    if (!dead.length) continue
    n += dead.length
    if (dead.length === stmt.specifiers.length) {
      s.edits.push({
        start: stmt.start,
        end: lineEnd(s.src, stmt.end),
        text: '',
      })
      continue
    }
    const keep = stmt.specifiers
      .filter((sp: AstNode) => !dead.includes(sp))
      .map((sp: AstNode) => s.text(sp))
    const open = s.src.indexOf('{', stmt.start)
    const close = s.src.indexOf('}', open)
    s.edits.push({
      start: open,
      end: close + 1,
      text: `{ ${keep.join(', ')} }`,
    })
  }
  return n
}

/** lineEnd returns the offset after the newline that ends the line containing i. */
function lineEnd(src: string, i: number): number {
  const nl = src.indexOf('\n', i)
  return nl < 0 ? src.length : nl + 1
}
