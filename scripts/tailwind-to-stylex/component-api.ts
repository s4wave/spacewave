import { parseSync, traverse } from '@babel/core'
import type { Node } from '@babel/types'
import { dirname, resolve, sep } from 'node:path'

// Edit retains source offsets until the complete component API change can be applied.
type Edit = { start: number; end: number; text: string }

/** prepareComponentStyles changes owned className inputs to composable StyleX styles. */
export function prepareComponentStyles(
  source: string,
  filename: string,
  options: { convertProps?: boolean; sharedComponents?: boolean } = {},
) {
  // Parse component bindings without executing declarations or changing unrelated source formatting.
  const ast = parseSync(source, {
    filename,
    configFile: false,
    babelrc: false,
    parserOpts: { plugins: ['typescript', 'jsx'] },
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

  // Collect owned JSX imports and reserve every existing local identifier.
  traverse(ast, {
    Identifier(path) {
      identifiers.add(path.node.name)
    },
    ImportDeclaration(path) {
      if (!options.sharedComponents) return
      const target = path.node.source.value
      if (
        !target.startsWith('@s4wave/web/ui/') &&
        !(
          target.startsWith('.') &&
          resolve(dirname(filename), target).startsWith(resolve('web/ui') + sep)
        )
      )
        return
      for (const specifier of path.node.specifiers)
        components.add(specifier.local.name)
    },
  })

  // Allocate a type import that cannot shadow a component or its existing styles.
  let typeName = 'ComponentStyles'
  for (let suffix = 2; identifiers.has(typeName); suffix++)
    typeName = `ComponentStyles${suffix}`
  const wrappedTypes: Node[] = []
  let converted = 0
  let needsType = false

  // Rename the parameter and its lexical references together; DOM className attributes stay compiler inputs.
  traverse(ast, {
    Function(path) {
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
          if (name) components.add(name)
          if (path.scope.hasOwnBinding('sx'))
            throw new Error(
              `${filename}: className and sx already share a component scope`,
            )
          const value = property.node.value
          const identifier =
            value.type === 'AssignmentPattern' ? value.left : value
          if (identifier.type !== 'Identifier') continue
          const binding = path.scope.getBinding(identifier.name)
          if (!binding) continue
          if (
            parameter.node.typeAnnotation?.type === 'TSTypeAnnotation' &&
            !wrappedTypes.includes(parameter.node.typeAnnotation.typeAnnotation)
          ) {
            const type = parameter.node.typeAnnotation.typeAnnotation
            wrappedTypes.push(type)
            change(
              type,
              `Omit<${source.slice(type.start!, type.end!)}, 'className'> & {sx?:${typeName}}`,
            )
            needsType = true
          }
          change(key, 'sx')
          change(identifier, 'sx')
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
      if (
        !path.parentPath.isTSInterfaceBody() &&
        !path.parentPath.parentPath?.isTSTypeAnnotation()
      )
        return
      change(key, 'sx')
      change(path.node.typeAnnotation.typeAnnotation, typeName)
      needsType = true
    },
    TSInterfaceDeclaration(path) {
      // Interface heritage excludes raw class names; its body declares the replacement input.
      let styled = false
      for (const heritage of path.node.extends ?? []) {
        const name = heritage.expression
        if (
          name.type !== 'TSQualifiedName' ||
          name.left.type !== 'Identifier' ||
          name.left.name !== 'React' ||
          ![
            'ComponentProps',
            'ComponentPropsWithRef',
            'HTMLAttributes',
            'ButtonHTMLAttributes',
          ].includes(name.right.name)
        )
          continue
        change(
          heritage,
          `Omit<${source.slice(heritage.start!, heritage.end!)}, 'className'>`,
        )
        styled = true
      }

      // Add the shared style type only to interfaces whose native prop inheritance changed.
      if (styled) {
        const offset = path.node.body.start! + 1
        edits.set(`interface:${offset}`, {
          start: offset,
          end: offset,
          text: `sx?:${typeName};`,
        })
        needsType = true
      }
    },
    TSTypeReference(path) {
      // Preserve the single enclosing parameter edit when it already owns this type.
      if (
        wrappedTypes.some(
          (type) =>
            path.node.start! >= type.start! && path.node.end! <= type.end!,
        )
      )
        return

      // Other native prop references expose the same composable style input.
      const name = path.node.typeName
      if (
        name.type !== 'TSQualifiedName' ||
        name.left.type !== 'Identifier' ||
        name.left.name !== 'React' ||
        ![
          'ComponentProps',
          'ComponentPropsWithRef',
          'HTMLAttributes',
          'ButtonHTMLAttributes',
        ].includes(name.right.name)
      )
        return
      change(
        path.node,
        `Omit<${source.slice(path.node.start!, path.node.end!)}, 'className'> & {sx?:${typeName}}`,
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
