import { parseSync, traverse } from '@babel/core'
import type { Node } from '@babel/types'
import { dirname, resolve, sep } from 'node:path'
import { readFile } from 'node:fs/promises'

// Edit retains source offsets until the complete component API change can be applied.
type Edit = { start: number; end: number; text: string }

const sharedApis = new Map<string, Promise<ReadonlySet<string>>>()
const sharedDependents = new Map<string, Set<string>>()

/** invalidateSharedStyleApis refreshes aliases of a component after its API changes on disk. */
export function invalidateSharedStyleApis(filename: string): void {
  // Invalidate the edited module's aliases and every barrel that forwards them.
  const visited = new Set<string>()
  const invalidate = (path: string) => {
    if (visited.has(path)) return
    visited.add(path)
    sharedApis.delete(path)
    for (const dependent of sharedDependents.get(path) ?? [])
      invalidate(dependent)
  }
  const base = resolve(filename).replace(/\.[cm]?[jt]sx?$/, '')
  for (const target of [
    base,
    ...(base.endsWith('/index') ? [dirname(base)] : []),
  ]) {
    for (const extension of ['', '.js', '.ts', '.tsx'])
      invalidate(target + extension)
  }
}

// sharedPath resolves owned source modules so style inputs can follow forwarding wrappers.
function sharedPath(target: string, filename: string): string | undefined {
  // Resolve project aliases and relative imports before reading a component's actual API.
  const roots = ['web', 'app', 'plugin']
  let path: string | undefined
  if (target.startsWith('.')) path = resolve(dirname(filename), target)
  else
    for (const root of roots) {
      const prefix = `@s4wave/${root}/`
      if (target.startsWith(prefix))
        path = resolve(root, target.slice(prefix.length))
    }

  // Keep assets and imports outside the owned source areas out of the API scan.
  if (!path || /\.(?![cm]?[jt]sx?$)[^./]+$/.test(path)) return
  if (roots.some((root) => path!.startsWith(resolve(root) + sep))) return path
}

// readSharedStyleApi follows value reexports through the module that owns each sx input.
async function readSharedStyleApi(
  path: string,
  ancestors: ReadonlySet<string> = new Set(),
): Promise<ReadonlySet<string>> {
  // A reexport cycle stops at the module already being resolved.
  if (ancestors.has(path)) return new Set()
  const cached = sharedApis.get(path)
  if (cached) return cached

  // Cache module work while its direct and forwarded exports are resolved.
  const pending = (async () => {
    // Resolve one source module, including directory entrypoints, without scanning the tree.
    const base = path.replace(/\.js$/, '')
    let contents: string | undefined
    let resolved = path
    for (const candidate of [
      base + '.tsx',
      base + '.ts',
      path,
      resolve(base, 'index.tsx'),
      resolve(base, 'index.ts'),
    ]) {
      try {
        contents = await readFile(candidate, 'utf8')
        resolved = candidate
        break
      } catch (error) {
        if (
          !['ENOENT', 'EISDIR', 'ENOTDIR'].includes(
            (error as NodeJS.ErrnoException).code ?? '',
          )
        )
          throw error
      }
    }
    if (contents === undefined)
      throw new Error(`Missing shared UI module: ${path}`)

    // Read owned sx inputs and their public export names from the same source snapshot.
    const { components } = prepareComponentStyles(contents, resolved, {
      convertProps: false,
    })
    const api = new Set(components)
    const parsed = parseSync(contents, {
      filename: resolved,
      configFile: false,
      babelrc: false,
      parserOpts: {
        plugins: /\.[cm]?ts$/.test(resolved)
          ? ['typescript']
          : ['typescript', 'jsx'],
      },
    })!
    for (const item of parsed.program.body) {
      if (item.type === 'ExportDefaultDeclaration') {
        const declaration = item.declaration
        const name =
          declaration.type === 'Identifier'
            ? declaration.name
            : declaration.type === 'FunctionDeclaration'
              ? declaration.id?.name
              : undefined
        if (name && components.has(name)) api.add('default')
      } else if (
        (item.type === 'ExportNamedDeclaration' ||
          item.type === 'ExportAllDeclaration') &&
        item.source &&
        item.exportKind !== 'type'
      ) {
        const child = sharedPath(item.source.value, resolved)
        if (!child) continue
        const dependents = sharedDependents.get(child) ?? new Set<string>()
        dependents.add(path)
        sharedDependents.set(child, dependents)
        const forwarded = await readSharedStyleApi(
          child,
          new Set([...ancestors, path]),
        )
        if (item.type === 'ExportAllDeclaration') {
          for (const name of forwarded) if (name !== 'default') api.add(name)
        } else
          for (const specifier of item.specifiers) {
            if (
              specifier.type !== 'ExportSpecifier' ||
              specifier.exportKind === 'type'
            )
              continue
            if (forwarded.has(specifier.local.name))
              api.add(
                specifier.exported.type === 'Identifier'
                  ? specifier.exported.name
                  : specifier.exported.value,
              )
          }
      } else if (item.type === 'ExportNamedDeclaration' && !item.source) {
        for (const specifier of item.specifiers) {
          if (
            specifier.type === 'ExportSpecifier' &&
            components.has(specifier.local.name)
          )
            api.add(
              specifier.exported.type === 'Identifier'
                ? specifier.exported.name
                : specifier.exported.value,
            )
        }
      }
    }
    return api
  })()
  sharedApis.set(path, pending)
  return pending
}

/** readSharedStyleApis reads existing sx exports instead of assuming every shared API has migrated. */
export async function readSharedStyleApis(source: string, filename: string) {
  // Parse imports without executing the modules whose public style APIs they use.
  const ast = parseSync(source, {
    filename,
    configFile: false,
    babelrc: false,
    parserOpts: {
      plugins: /\.[cm]?ts$/.test(filename)
        ? ['typescript']
        : ['typescript', 'jsx'],
    },
  })!
  const imports = new Map<string, ReadonlySet<string>>()
  for (const statement of ast.program.body) {
    if (
      statement.type !== 'ImportDeclaration' ||
      statement.importKind === 'type'
    )
      continue
    const target = statement.source.value
    const path = sharedPath(target, filename)
    if (path) imports.set(target, await readSharedStyleApi(path))
  }
  return imports
}

/** prepareComponentStyles changes owned className inputs to composable StyleX styles. */
export function prepareComponentStyles(
  source: string,
  filename: string,
  options: {
    convertProps?: boolean
    sharedComponents?: ReadonlyMap<string, ReadonlySet<string>>
  } = {},
) {
  // Parse component bindings without executing declarations or changing unrelated source formatting.
  const ast = parseSync(source, {
    filename,
    configFile: false,
    babelrc: false,
    parserOpts: {
      plugins: /\.[cm]?ts$/.test(filename)
        ? ['typescript']
        : ['typescript', 'jsx'],
    },
  })!
  const edits = new Map<string, Edit>()
  const identifiers = new Set<string>()
  const components = new Set<string>()
  const change = (node: Node, text: string) => {
    edits.set(`${node.start}:${node.end}`, {
      start: node.start!,
      end: node.end!,
      text,
    })
  }

  // Explicit function types expose sx even when a factory creates the component value.
  const styledTypes = new Set<string>()
  for (const item of ast.program.body) {
    const declaration =
      item.type === 'ExportNamedDeclaration' ? item.declaration : item
    if (
      declaration?.type !== 'TSTypeAliasDeclaration' ||
      declaration.typeAnnotation.type !== 'TSFunctionType'
    )
      continue
    const type = declaration.typeAnnotation.parameters[0]?.typeAnnotation
    if (type?.type !== 'TSTypeAnnotation') continue
    const input = type.typeAnnotation
    const parts = input.type === 'TSIntersectionType' ? input.types : [input]
    if (
      parts.some(
        (part) =>
          part.type === 'TSTypeLiteral' &&
          part.members.some(
            (member) =>
              member.type === 'TSPropertySignature' &&
              member.key.type === 'Identifier' &&
              member.key.name === 'sx',
          ),
      )
    )
      styledTypes.add(declaration.id.name)
  }

  // Collect owned JSX imports and reserve every existing local identifier.
  traverse(ast, {
    VariableDeclarator(path) {
      const id = path.node.id
      const type =
        id.type === 'Identifier' &&
        id.typeAnnotation?.type === 'TSTypeAnnotation'
          ? id.typeAnnotation.typeAnnotation
          : undefined
      if (
        id.type === 'Identifier' &&
        type?.type === 'TSTypeReference' &&
        type.typeName.type === 'Identifier' &&
        styledTypes.has(type.typeName.name)
      )
        components.add(id.name)
    },
    Identifier(path) {
      identifiers.add(path.node.name)
    },
    ImportDeclaration(path) {
      const api = options.sharedComponents?.get(path.node.source.value)
      if (!api) return
      for (const specifier of path.node.specifiers) {
        if (specifier.type === 'ImportNamespaceSpecifier') {
          for (const name of api)
            components.add(`${specifier.local.name}.${name}`)
        } else {
          const name =
            specifier.type === 'ImportDefaultSpecifier'
              ? 'default'
              : specifier.imported.type === 'Identifier'
                ? specifier.imported.name
                : specifier.imported.value
          if (api.has(name)) components.add(specifier.local.name)
        }
      }
    },
  })

  // Allocate a type import that cannot shadow a component or its existing styles.
  let typeName = 'ComponentStyles'
  for (let suffix = 2; identifiers.has(typeName); suffix++)
    typeName = `ComponentStyles${suffix}`
  const wrappedTypes: Node[] = []
  const ownedProps = new Set<string>()
  let converted = 0
  let needsType = false

  // Rename the parameter and its lexical references together; DOM className attributes stay compiler inputs.
  traverse(ast, {
    Function(path) {
      // A partially converted component keeps its existing two inputs for the hand pass.
      if (
        path.scope.hasOwnBinding('sx') &&
        path.node.params.some(
          (parameter) =>
            parameter.type === 'ObjectPattern' &&
            parameter.properties.some(
              (property) =>
                property.type === 'ObjectProperty' &&
                property.key.type === 'Identifier' &&
                property.key.name === 'className',
            ),
        )
      )
        return
      for (const parameter of path.get('params')) {
        if (!parameter.isObjectPattern()) continue
        for (const property of parameter.get('properties')) {
          if (!property.isObjectProperty() || property.node.computed) continue
          const key = property.node.key
          if (key.type !== 'Identifier') continue
          const component =
            path.node.type === 'FunctionDeclaration'
              ? path.node.id?.name
              : path.findParent((parent) => parent.isVariableDeclarator())?.node
          const name =
            typeof component === 'string'
              ? component
              : component?.type === 'VariableDeclarator' &&
                  component.id.type === 'Identifier'
                ? component.id.name
                : undefined
          if (key.name === 'sx' && name) components.add(name)
          if (key.name !== 'className' || options.convertProps === false)
            continue
          const value = property.node.value
          const identifier =
            value.type === 'AssignmentPattern' ? value.left : value
          if (identifier.type !== 'Identifier') continue
          if (
            value.type === 'AssignmentPattern' &&
            (value.right.type !== 'StringLiteral' || value.right.value !== '')
          )
            continue
          const binding = path.scope.getBinding(identifier.name)
          if (!binding) continue
          if (name) components.add(name)
          if (
            parameter.node.typeAnnotation?.type === 'TSTypeAnnotation' &&
            !wrappedTypes.includes(parameter.node.typeAnnotation.typeAnnotation)
          ) {
            const type = parameter.node.typeAnnotation.typeAnnotation
            if (
              type.type === 'TSTypeReference' &&
              type.typeName.type === 'Identifier'
            )
              ownedProps.add(type.typeName.name)
            wrappedTypes.push(type)
            change(
              type,
              `Omit<${source.slice(type.start!, type.end!)}, 'className'> & {sx?:${typeName};className?:never}`,
            )
            needsType = true
          }
          if (value.type !== 'AssignmentPattern' || key.start !== value.start)
            change(key, 'sx')
          change(value.type === 'AssignmentPattern' ? value : identifier, 'sx')
          for (const reference of binding.referencePaths) {
            const parent = reference.parentPath
            change(
              reference.node,
              parent?.isObjectProperty() && parent.node.shorthand
                ? `${identifier.name}: sx`
                : 'sx',
            )
          }
          converted++
        }
      }
    },
  })
  if (!converted) return { source, converted, components }

  // Owned prop records exclude raw class strings while DOM and Radix receive compiled props at their leaves.
  traverse(ast, {
    TSPropertySignature(path) {
      // Parameter annotations already carry their complete replacement type.
      if (
        wrappedTypes.some(
          (type) =>
            path.node.start! >= type.start! && path.node.end! <= type.end!,
        )
      )
        return

      // Change explicit component prop fields while leaving nested icon and library contracts alone.
      const key = path.node.key
      if (
        key.type !== 'Identifier' ||
        key.name !== 'className' ||
        path.node.typeAnnotation?.typeAnnotation.type !== 'TSStringKeyword'
      )
        return
      const owner = path.findParent((parent) =>
        parent.isTSInterfaceDeclaration(),
      )
      if (
        !owner?.isTSInterfaceDeclaration() ||
        !ownedProps.has(owner.node.id.name)
      )
        return
      change(key, 'sx')
      change(path.node.typeAnnotation.typeAnnotation, typeName)
      needsType = true
    },
    TSInterfaceDeclaration(path) {
      // Keep unrelated native and pass-through prop records intact.
      if (!ownedProps.has(path.node.id.name)) return

      // Interface heritage excludes raw class names; its body declares the replacement input.
      let styled = false
      for (const heritage of path.node.extends ?? []) {
        change(
          heritage,
          `Omit<${source.slice(heritage.start!, heritage.end!)}, 'className'>`,
        )
        styled = true
      }

      // Forbid raw classes even through JSX spreads, and add the owned style input when inherited props need it.
      const hasStyleField = path.node.body.body.some(
        (member) =>
          member.type === 'TSPropertySignature' &&
          member.key.type === 'Identifier' &&
          ['className', 'sx'].includes(member.key.name),
      )
      const offset = path.node.body.start! + 1
      edits.set(`interface:${offset}`, {
        start: offset,
        end: offset,
        text: `className?:never;${styled && !hasStyleField ? `sx?:${typeName};` : ''}`,
      })
      needsType = true
    },
    TSTypeAliasDeclaration(path) {
      // Only an owned component's alias changes; pass-through and library props keep their contracts.
      if (!ownedProps.has(path.node.id.name)) return
      const type = path.node.typeAnnotation
      wrappedTypes.push(type)
      change(
        type,
        `Omit<${source.slice(type.start!, type.end!)}, 'className'> & {sx?:${typeName};className?:never}`,
      )
      needsType = true
    },
  })

  // Apply each lexical edit once, then add the type-only import outside the runtime compiler namespace.
  for (const edit of [...edits.values()].sort(
    (left, right) => right.start - left.start,
  ))
    source = source.slice(0, edit.start) + edit.text + source.slice(edit.end)
  if (needsType)
    source = `import type {StyleXStyles as ${typeName}} from '@stylexjs/stylex'\n${source}`
  return { source, converted, components }
}
