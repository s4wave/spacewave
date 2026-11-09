import { readFileSync } from 'node:fs'
import { parseSync, traverse, types as t, type NodePath } from '@babel/core'

// fragmentModulePattern names modules whose exports are plain StyleX property groups.
export const fragmentModulePattern = /\.fragments\.[cm]?[jt]s$/

// fragmentSpecifierPattern keeps resolution off every other import.
export const fragmentSpecifierPattern = /\.fragments(?:\.[cm]?[jt]s)?$/

// stylexModulePattern names the token modules a fragment value may reference.
const stylexModulePattern = /\.stylex(?:\.const)?\.[cm]?[jt]s$/

/** ResolveFile maps an import specifier to a source file, or undefined when external. */
export type ResolveFile = (
  specifier: string,
  importer: string,
) => Promise<string | undefined>

// Binding is one import a fragment module or consumer holds.
type Binding =
  | { kind: 'namespace'; file: string }
  | { kind: 'named'; file: string; name: string }

// FragmentModule is one parsed fragment source and its top-level bindings.
interface FragmentModule {
  file: string
  exports: Map<string, t.Expression>
  reexports: Map<string, Binding>
  starExports: string[]
  locals: Map<string, t.Expression>
  imports: Map<string, { specifier: string; imported: string | null }>
}

/**
 * inlineFragments replaces `...ns.group.entry` spreads of fragment exports with
 * their properties, so the StyleX compiler sees one static object per rule.
 * Token references inside a fragment become imports of the same token modules.
 * Returns the fragment files the source depends on.
 */
export async function inlineFragments(
  ast: t.File,
  file: string,
  resolveFile: ResolveFile,
): Promise<string[]> {
  // Find the consumer's fragment imports; most sources have none.
  const bindings = new Map<string, Binding>()
  const declarations = new Map<string, t.ImportDeclaration>()
  for (const node of ast.program.body) {
    if (node.type !== 'ImportDeclaration' || node.importKind === 'type')
      continue
    if (!fragmentSpecifierPattern.test(node.source.value)) continue
    const resolved = await resolveFile(node.source.value, file)
    if (!resolved || !fragmentModulePattern.test(resolved)) continue
    for (const spec of node.specifiers) {
      if (spec.type === 'ImportNamespaceSpecifier')
        bindings.set(spec.local.name, { kind: 'namespace', file: resolved })
      else if (spec.type === 'ImportSpecifier')
        bindings.set(spec.local.name, {
          kind: 'named',
          file: resolved,
          name: t.isIdentifier(spec.imported)
            ? spec.imported.name
            : spec.imported.value,
        })
      else throw new Error(`${file}: fragment modules have no default export`)
      declarations.set(spec.local.name, node)
    }
  }
  if (!bindings.size) return []

  // Load fragment modules on demand; resolution is async, so gather spreads first.
  const modules = new Map<string, FragmentModule>()
  const used = new Set<string>()
  const load = async (path: string): Promise<FragmentModule> => {
    let m = modules.get(path)
    if (!m) {
      m = await parseFragmentModule(path, resolveFile)
      modules.set(path, m)
      used.add(path)
    }
    return m
  }

  // lookup follows exports and object keys to the fragment object for a path.
  const lookup = async (
    binding: Binding,
    keys: string[],
  ): Promise<{ module: FragmentModule; object: t.ObjectExpression }> => {
    let module = await load(binding.file)
    let rest = keys
    if (binding.kind === 'named') rest = [binding.name, ...keys]
    let expr: t.Expression | undefined
    while (!expr) {
      const [head, ...tail] = rest
      if (head === undefined)
        throw new Error(`${file}: spread a fragment entry, not a module`)
      const own = module.exports.get(head)
      if (own) {
        expr = own
        rest = tail
        break
      }
      const re = module.reexports.get(head)
      if (re?.kind === 'named') {
        module = await load(re.file)
        rest = [re.name, ...tail]
        continue
      }
      if (re?.kind === 'namespace') {
        module = await load(re.file)
        rest = tail
        continue
      }
      let found = false
      for (const star of module.starExports) {
        const next = await load(star)
        if (next.exports.has(head) || next.reexports.has(head)) {
          module = next
          found = true
          break
        }
      }
      if (!found)
        throw new Error(`${file}: ${module.file} does not export ${head}`)
    }
    for (const key of rest) {
      const object = unwrap(expr)
      if (!t.isObjectExpression(object))
        throw new Error(`${file}: fragment ${keys.join('.')} is not an object`)
      const prop = object.properties.find(
        (p): p is t.ObjectProperty =>
          t.isObjectProperty(p) && !p.computed && propertyKey(p) === key,
      )
      if (!prop || !t.isExpression(prop.value))
        throw new Error(`${file}: fragment has no entry ${keys.join('.')}`)
      expr = prop.value
    }
    const object = unwrap(expr)
    if (!t.isObjectExpression(object))
      throw new Error(`${file}: fragment ${keys.join('.')} is not an object`)
    return { module, object }
  }

  // Collect spreads of fragment paths inside object literals.
  const spreads: {
    path: NodePath<t.SpreadElement>
    binding: Binding
    keys: string[]
  }[] = []
  traverse(ast, {
    SpreadElement(path) {
      if (!t.isObjectExpression(path.parent)) return
      const chain = memberChain(path.node.argument)
      if (!chain) return
      const binding = bindings.get(chain[0]!)
      if (!binding || path.scope.getBinding(chain[0]!)?.kind !== 'module')
        return
      spreads.push({ path, binding, keys: chain.slice(1) })
    },
  })

  // Splice each fragment's properties, importing the tokens they reference.
  const added = new Map<string, string>()
  const addImport = (source: string, imported: string | null) => {
    const key = `${source}\0${imported ?? '*'}`
    let local = added.get(key)
    if (!local) {
      local = `__stylexFragment${added.size}`
      added.set(key, local)
      ast.program.body.unshift(
        t.importDeclaration(
          [
            imported === null
              ? t.importNamespaceSpecifier(t.identifier(local))
              : t.importSpecifier(t.identifier(local), t.identifier(imported)),
          ],
          t.stringLiteral(source),
        ),
      )
    }
    return local
  }
  for (const { path, binding, keys } of spreads) {
    const { module, object } = await lookup(binding, keys)
    const props = await Promise.all(
      object.properties.map(async (p) => {
        if (!t.isObjectProperty(p))
          throw new Error(
            `${module.file}: fragment entries hold plain properties`,
          )
        return materialize(t.cloneNode(p, true), module, addImport, resolveFile)
      }),
    )
    path.replaceWithMultiple(props)
  }

  // Drop consumer imports the inlining left unreferenced.
  traverse(ast, {
    Program(path) {
      path.scope.crawl()
      for (const [local, decl] of declarations) {
        if (path.scope.getBinding(local)?.referenced) continue
        decl.specifiers = decl.specifiers.filter((s) => s.local.name !== local)
      }
      const emptied = new Set(
        [...declarations.values()].filter((d) => !d.specifiers.length),
      )
      ast.program.body = ast.program.body.filter(
        (n) => !emptied.has(n as t.ImportDeclaration),
      )
      path.stop()
    },
  })
  return [...used]
}

/** materialize rewrites a cloned fragment node so it reads only literals and token imports. */
async function materialize<T extends t.Node>(
  node: T,
  module: FragmentModule,
  addImport: (source: string, imported: string | null) => string,
  resolveFile: ResolveFile,
  depth = 0,
): Promise<T> {
  if (depth > 32) throw new Error(`${module.file}: fragment constants recurse`)
  // Replace identifiers bottom-up; keys and static member names stay as written.
  const visit = async (
    n: t.Node,
    parent: t.Node | null,
    field: string,
  ): Promise<t.Node> => {
    if (t.isIdentifier(n)) {
      if (
        parent &&
        t.isObjectProperty(parent) &&
        field === 'key' &&
        !parent.computed
      )
        return n
      if (
        parent &&
        t.isMemberExpression(parent) &&
        field === 'property' &&
        !parent.computed
      )
        return n
      const local = module.locals.get(n.name)
      if (local)
        return materialize(
          t.cloneNode(local, true),
          module,
          addImport,
          resolveFile,
          depth + 1,
        )
      const imp = module.imports.get(n.name)
      if (imp) {
        const resolved = await resolveFile(imp.specifier, module.file)
        if (!resolved || !stylexModulePattern.test(resolved))
          throw new Error(
            `${module.file}: ${n.name} is not a StyleX token import`,
          )
        return t.identifier(addImport(resolved, imp.imported))
      }
      throw new Error(`${module.file}: ${n.name} is not static`)
    }
    for (const key of t.VISITOR_KEYS[n.type] ?? []) {
      const fields = n as unknown as Record<string, unknown>
      const value = fields[key] as t.Node | (t.Node | null)[] | null
      if (Array.isArray(value)) {
        for (let i = 0; i < value.length; i++) {
          const item = value[i]
          if (item) value[i] = await visit(item, n, key)
        }
      } else if (value && typeof value.type === 'string') {
        fields[key] = await visit(value, n, key)
      }
    }
    return n
  }
  return (await visit(node, null, '')) as T
}

/** parseFragmentModule reads a fragment source's exports, constants and imports. */
async function parseFragmentModule(
  file: string,
  resolveFile: ResolveFile,
): Promise<FragmentModule> {
  const ast = parseSync(readFileSync(file, 'utf8'), {
    filename: file,
    configFile: false,
    babelrc: false,
    parserOpts: { plugins: ['typescript'] },
  })!
  const m: FragmentModule = {
    file,
    exports: new Map(),
    reexports: new Map(),
    starExports: [],
    locals: new Map(),
    imports: new Map(),
  }
  const constants = (
    decl: t.VariableDeclaration,
    into: Map<string, t.Expression>,
  ) => {
    if (decl.kind !== 'const') throw new Error(`${file}: fragments use const`)
    for (const d of decl.declarations)
      if (t.isIdentifier(d.id) && d.init) into.set(d.id.name, unwrap(d.init))
  }
  for (const node of ast.program.body) {
    if (t.isImportDeclaration(node)) {
      if (node.importKind === 'type') continue
      for (const s of node.specifiers)
        m.imports.set(s.local.name, {
          specifier: node.source.value,
          imported: t.isImportSpecifier(s)
            ? t.isIdentifier(s.imported)
              ? s.imported.name
              : s.imported.value
            : null,
        })
    } else if (t.isVariableDeclaration(node)) {
      constants(node, m.locals)
    } else if (t.isExportNamedDeclaration(node)) {
      if (node.exportKind === 'type') continue
      if (t.isVariableDeclaration(node.declaration)) {
        constants(node.declaration, m.exports)
        constants(node.declaration, m.locals)
      } else if (node.source) {
        const target = await resolveFile(node.source.value, file)
        if (!target)
          throw new Error(`${file}: cannot resolve ${node.source.value}`)
        for (const s of node.specifiers) {
          const exported = t.isIdentifier(s.exported)
            ? s.exported.name
            : s.exported.value
          if (t.isExportNamespaceSpecifier(s))
            m.reexports.set(exported, { kind: 'namespace', file: target })
          else if (t.isExportSpecifier(s))
            m.reexports.set(exported, {
              kind: 'named',
              file: target,
              name: s.local.name,
            })
        }
      }
    } else if (t.isExportAllDeclaration(node) && node.exportKind !== 'type') {
      const target = await resolveFile(node.source.value, file)
      if (!target)
        throw new Error(`${file}: cannot resolve ${node.source.value}`)
      m.starExports.push(target)
    }
  }
  return m
}

/** unwrap strips TypeScript assertions that do not change a value. */
function unwrap(expr: t.Expression): t.Expression {
  while (
    t.isTSAsExpression(expr) ||
    t.isTSSatisfiesExpression(expr) ||
    t.isTSNonNullExpression(expr) ||
    t.isParenthesizedExpression(expr)
  )
    expr = expr.expression
  return expr
}

/** propertyKey reads a static object key. */
function propertyKey(p: t.ObjectProperty): string | undefined {
  if (t.isIdentifier(p.key)) return p.key.name
  if (t.isStringLiteral(p.key)) return p.key.value
  if (t.isNumericLiteral(p.key)) return String(p.key.value)
  return undefined
}

/** memberChain reads `a.b['c']` as ['a', 'b', 'c'], or undefined for other shapes. */
function memberChain(expr: t.Node): string[] | undefined {
  const keys: string[] = []
  while (t.isMemberExpression(expr)) {
    const p = expr.property
    if (!expr.computed && t.isIdentifier(p)) keys.unshift(p.name)
    else if (t.isStringLiteral(p)) keys.unshift(p.value)
    else if (t.isNumericLiteral(p)) keys.unshift(String(p.value))
    else return undefined
    expr = expr.object
  }
  if (!t.isIdentifier(expr)) return undefined
  keys.unshift(expr.name)
  return keys
}
