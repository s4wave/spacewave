import { readFileSync } from 'node:fs'

// skinClasses comes from CSS that styles opaque vendor and bootstrap markup.
const skinClasses = new Set(['dark', 'light'])
for (const file of [
  '../web/style/app.css',
  '../web/style/sonner.css',
  '../web/style/flexlayout/base.css',
  '../web/style/flexlayout/flexlayout.css',
  '../app/landing/AnimatedLogo.css',
  '../app/docs/docs-prose.css',
  '../app/blog/blog-prose.css',
  '../plugin/notes/blog/blog-prose.css',
  '../app/loading/boot-loading-critical.tsx',
  '../web/ui/loading/loading-screen-style.ts',
]) {
  const css = readFileSync(new URL(file, import.meta.url), 'utf8')
  for (const match of css.matchAll(/\.([a-zA-Z_][\w-]*)(?=[\s.#:[>,{])/g)) {
    skinClasses.add(match[1])
  }
}

// classLiterals follows class compositions but leaves compiled StyleX calls opaque.
function* classLiterals(node, scope, visited = new Set()) {
  if (!node || visited.has(node)) return
  visited.add(node)
  switch (node.type) {
    case 'Literal':
      if (typeof node.value === 'string') yield node
      return
    case 'TemplateLiteral':
      for (const part of node.quasis) {
        if (part.value.cooked?.trim()) {
          yield { ...part, value: part.value.cooked }
        }
      }
      for (const expression of node.expressions) {
        yield* classLiterals(expression, scope, visited)
      }
      return
    case 'Identifier': {
      // Local class constants receive the same check as strings written at the DOM root.
      let current = scope
      while (current) {
        const variable = current.variables.find(
          (entry) => entry.name === node.name,
        )
        if (variable) {
          for (const definition of variable.defs) {
            yield* classLiterals(definition.node.init, current, visited)
          }
          return
        }
        current = current.upper
      }
      return
    }
    case 'ConditionalExpression':
      yield* classLiterals(node.consequent, scope, visited)
      yield* classLiterals(node.alternate, scope, visited)
      return
    case 'LogicalExpression':
      yield* classLiterals(node.left, scope, visited)
      yield* classLiterals(node.right, scope, visited)
      return
    case 'ArrayExpression':
      for (const element of node.elements) {
        yield* classLiterals(element, scope, visited)
      }
      return
    case 'CallExpression':
      if (
        node.callee.type === 'MemberExpression' &&
        ['join', 'filter'].includes(node.callee.property.name)
      ) {
        yield* classLiterals(node.callee.object, scope, visited)
      }
      return
    case 'JSXExpressionContainer':
      yield* classLiterals(node.expression, scope, visited)
      return
  }
}

// nativeClasses keeps utility strings out of DOM, library and imperative class producers.
const nativeClasses = {
  meta: {
    type: 'problem',
    schema: [],
    messages: {
      utility:
        'Use sx for "{{classes}}". Literal classes must belong to an opaque CSS skin.',
    },
  },
  create(context) {
    // Check each literal once, even when a constant reaches several DOM elements.
    const reported = new Set()
    const themeTypes = new Set()
    const check = (node) => {
      for (const literal of classLiterals(
        node,
        context.sourceCode.getScope(node),
      )) {
        const invalid = literal.value
          .split(/\s+/)
          .filter((value) => value && !skinClasses.has(value))
        if (invalid.length && !reported.has(literal)) {
          context.report({
            node: literal,
            messageId: 'utility',
            data: { classes: invalid.join(' ') },
          })
          reported.add(literal)
        }
      }
    }
    // Lexical owns its DOM classes through a nested theme instead of JSX props.
    const checkTheme = (node) => {
      if (node.type === 'ObjectExpression') {
        for (const property of node.properties) {
          if (property.type === 'Property') checkTheme(property.value)
        }
      } else check(node)
    }
    return {
      ImportDeclaration(node) {
        if (node.source.value !== 'lexical') return
        for (const specifier of node.specifiers) {
          if (specifier.imported?.name === 'EditorThemeClasses')
            themeTypes.add(specifier.local.name)
        }
      },
      VariableDeclarator(node) {
        const type = node.id.typeAnnotation?.typeAnnotation
        if (
          node.init &&
          type?.type === 'TSTypeReference' &&
          themeTypes.has(type.typeName.name)
        )
          checkTheme(node.init)
      },
      JSXAttribute(node) {
        if (node.name.name === 'className' && node.value) check(node.value)
      },
      Property(node) {
        if ((node.key.name ?? node.key.value) === 'className') check(node.value)
      },
      AssignmentExpression(node) {
        if (
          node.left.type === 'MemberExpression' &&
          (node.left.property.name ?? node.left.property.value) === 'className'
        )
          check(node.right)
      },
    }
  },
}

export default { rules: { 'native-classes': nativeClasses } }
