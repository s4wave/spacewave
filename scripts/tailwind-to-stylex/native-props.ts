import { isDefaultObjectParameter } from './parameters.js'
import { parseSync, traverse } from '@babel/core'

type Edit = { start: number; end: number; text: string }

/** preserveNativeProps keeps dynamic sx variables beside forwarded native inline styles. */
export function preserveNativeProps(source: string, filename: string): string {
  // Read the owned sx parameter and its actual forwarded rest binding.
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
  const names = new Set<string>()
  let imported: string | undefined
  traverse(ast, {
    Identifier(path) {
      names.add(path.node.name)
    },
    ImportDeclaration(path) {
      if (path.node.source.value !== '@s4wave/web/style/props.js') return
      for (const item of path.node.specifiers)
        if (
          item.type === 'ImportSpecifier' &&
          item.imported.type === 'Identifier' &&
          item.imported.name === 'stylexProps'
        )
          imported = item.local.name
    },
  })

  // Reuse an existing helper import and avoid shadowing any application binding.
  let binding = imported ?? 'stylexProps'
  for (let suffix = 2; !imported && names.has(binding); suffix++)
    binding = `stylexProps${suffix}`
  const edits: Edit[] = []
  const text = (node: { start?: number | null; end?: number | null }) =>
    source.slice(node.start!, node.end!)
  traverse(ast, {
    JSXOpeningElement(path) {
      // Only a component that owns sx may replace its forwarded native-property composition.
      const fn = path.getFunctionParent()?.node
      if (!fn || !('params' in fn)) return
      const input = fn.params
        .map((parameter) =>
          isDefaultObjectParameter(parameter) ? parameter.left : parameter,
        )
        .find(
          (p) =>
            p.type === 'ObjectPattern' &&
            p.properties.some(
              (q) =>
                q.type === 'ObjectProperty' &&
                q.key.type === 'Identifier' &&
                q.key.name === 'sx',
            ),
        )
      if (input?.type !== 'ObjectPattern') return
      const rest = input.properties.find((p) => p.type === 'RestElement')
      if (rest?.type !== 'RestElement' || rest.argument.type !== 'Identifier')
        return

      // Adjacent attributes preserve their existing evaluation order and every unrelated native prop.
      const native =
        path.node.name.type === 'JSXIdentifier' &&
        /^[a-z]/.test(path.node.name.name)
      const attrs = path.node.attributes
      for (let index = 0; index < attrs.length - 1; index++) {
        const current = attrs[index]!,
          next = attrs[index + 1]!
        if (
          next.type !== 'JSXSpreadAttribute' ||
          next.argument.type !== 'Identifier' ||
          next.argument.name !== rest.argument.name
        )
          continue
        let styles: string | undefined
        if (
          native &&
          current.type === 'JSXAttribute' &&
          current.name.type === 'JSXIdentifier' &&
          current.name.name === 'sx' &&
          current.value?.type === 'JSXExpressionContainer'
        )
          styles = text(current.value.expression)
        if (
          current.type === 'JSXSpreadAttribute' &&
          current.argument.type === 'CallExpression'
        ) {
          const call = current.argument,
            callee = call.callee
          if (
            callee.type !== 'MemberExpression' ||
            callee.object.type !== 'Identifier' ||
            callee.property.type !== 'Identifier' ||
            callee.property.name !== 'props'
          )
            continue
          const runtime = path.scope.getBinding(callee.object.name)?.path
          if (
            !runtime?.isImportNamespaceSpecifier() ||
            !runtime.parentPath.isImportDeclaration() ||
            runtime.parentPath.node.source.value !== '@stylexjs/stylex'
          )
            continue
          styles =
            call.arguments.length === 1
              ? text(call.arguments[0]!)
              : `[${call.arguments.map(text).join(',')}]`
        }
        if (styles)
          edits.push({
            start: current.start!,
            end: next.end!,
            text: `{...${binding}(${styles},${rest.argument.name})}`,
          })
      }
    },
  })

  // Keep offsets stable until every composition decision is complete.
  if (!edits.length) return source
  for (const edit of edits.sort((a, b) => b.start - a.start))
    source = source.slice(0, edit.start) + edit.text + source.slice(edit.end)
  return imported
    ? source
    : `import {${binding === 'stylexProps' ? binding : 'stylexProps as ' + binding}} from '@s4wave/web/style/props.js'\n${source}`
}
