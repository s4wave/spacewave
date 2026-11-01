import { parseSync, traverse } from '@babel/core'
import type { Node, CallExpression } from '@babel/types'
import { dirname, relative, resolve } from 'node:path'

import type { createOracle, StyleRule } from './oracle.js'
import { compileVariants } from './variants.js'

/** MigrationSite records a class expression that needs a component-level decision. */
export interface MigrationSite {
  line: number
  reason: string
  source: string
}

/** MigrationResult contains deterministic source and the remaining hand-pass inventory. */
export interface MigrationResult {
  source: string
  converted: number
  remaining: MigrationSite[]
}

type Syntax = Node & { [key: string]: unknown }
type Edit = { start: number; end: number; text: string }
type Oracle = Awaited<ReturnType<typeof createOracle>>

/** TransformOptions selects component APIs whose callers already accept StyleX styles. */
export interface TransformOptions {
  components?: ReadonlySet<string>
  componentProps?: boolean
  onlyComponents?: boolean
}

// mergeStyle retains independent conditions while later utilities replace the same declaration.
function mergeStyle(target: StyleRule, source: StyleRule): void {
  for (const [key, value] of Object.entries(source)) {
    if (value !== null && typeof value === 'object') {
      const previous = target[key]
      if (previous !== null && typeof previous === 'object')
        mergeStyle(previous, value)
      else target[key] = structuredClone(value)
    } else target[key] = value
  }
}

// propertyStyles puts CSS conditions inside their properties, as the StyleX type contract requires.
function propertyStyles(style: StyleRule): StyleRule {
  // Accumulate declarations at their condition path, retaining pseudo-elements as separate targets.
  const output: StyleRule = {}
  const walk = (source: StyleRule, target: StyleRule, conditions: string[]) => {
    for (const [property, raw] of Object.entries(source)) {
      if (raw !== null && typeof raw === 'object') {
        if (property.startsWith('::')) {
          const pseudo = (target[property] ??= {}) as StyleRule
          walk(raw, pseudo, conditions)
        } else walk(raw, target, [...conditions, property])
        continue
      }

      // Numeric CSS values must stay numeric for closed types such as z-index and line-clamp.
      const value =
        typeof raw === 'string' &&
        !property.startsWith('--') &&
        /^-?(?:\d+\.?\d*|\.\d+)$/.test(raw)
          ? Number(raw)
          : raw
      const previous = target[property]
      if (!conditions.length) {
        if (previous !== null && typeof previous === 'object')
          previous.default = value
        else target[property] = value
        continue
      }

      // A conditional-only property clears earlier StyleX values; ambiguous compositions stay manual below.
      const root =
        previous !== null && typeof previous === 'object'
          ? previous
          : { default: previous ?? null }
      target[property] = root
      let branch = root
      for (const [index, condition] of conditions.entries()) {
        if (index === conditions.length - 1) branch[condition] = value
        else {
          const previous = branch[condition]
          branch = branch[condition] =
            previous !== null && typeof previous === 'object'
              ? previous
              : { default: previous ?? null }
        }
      }
    }
  }
  walk(style, output, [])
  return output
}

// visit walks the parser's syntax nodes without traversing comments or source locations.
function visit(node: unknown, callback: (node: Syntax) => void): void {
  // A child can be a node, a node list or scalar parser metadata.
  if (!node || typeof node !== 'object') return
  if (Array.isArray(node)) {
    for (const child of node) visit(child, callback)
    return
  }

  // Parser nodes form a tree; metadata and comment objects are not syntax children.
  if (!('type' in node) || typeof node.type !== 'string') return
  callback(node as Syntax)
  for (const [key, value] of Object.entries(node)) {
    if (
      !['loc', 'leadingComments', 'trailingComments', 'innerComments'].includes(
        key,
      )
    )
      visit(value, callback)
  }
}

/** transform migrates supported JSX class expressions without executing application code. */
export function transform(
  source: string,
  filename: string,
  oracle: Oracle,
  options: TransformOptions = {},
  allowVariants = true,
): MigrationResult {
  // Retain original offsets so untouched application code stays byte-for-byte unchanged.
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
  const edits: Edit[] = []
  let converted = 0
  const remaining: MigrationSite[] = []
  const namespaces = new Map<string, { name: string; style: StyleRule }>()

  // Only the imported class combiner participates; similarly spelled local functions keep their contract.
  const classHelpers = new Set(
    ast.program.body.flatMap((node) =>
      node.type === 'ImportDeclaration' &&
      /(?:^cnfast$|(?:^|\/)style\/utils(?:\.js)?$)/.test(node.source.value)
        ? node.specifiers
            .filter(
              (specifier) =>
                specifier.type === 'ImportSpecifier' &&
                specifier.imported.type === 'Identifier' &&
                specifier.imported.name === 'cn',
            )
            .map((specifier) => specifier.local.name)
        : [],
    ),
  )

  // Allocate compiler bindings against every identifier before inserting declarations.
  const variantHelpers = new Set(
    ast.program.body.flatMap((node) =>
      node.type === 'ImportDeclaration' &&
      node.source.value === 'class-variance-authority'
        ? node.specifiers
            .filter(
              (specifier) =>
                specifier.type === 'ImportSpecifier' &&
                specifier.imported.type === 'Identifier' &&
                specifier.imported.name === 'cva',
            )
            .map((specifier) => specifier.local.name)
        : [],
    ),
  )
  const variantDefinitions = new Map<
    string,
    { start: number; end: number; source: string }
  >()
  const identifiers = new Set<string>()
  visit(ast, (node) => {
    if (node.type === 'Identifier') identifiers.add(node.name as string)
  })
  const unique = (base: string) => {
    let name = base
    for (let suffix = 2; identifiers.has(name); suffix++)
      name = `${base}${suffix}`
    identifiers.add(name)
    return name
  }
  const prefix = unique('stylexStyles')

  // Reuse the existing runtime namespace and import only the token groups the rules consume.
  const existingStylex = ast.program.body.flatMap((node) =>
    node.type === 'ImportDeclaration' &&
    node.source.value === '@stylexjs/stylex'
      ? node.specifiers
          .filter((specifier) => specifier.type === 'ImportNamespaceSpecifier')
          .map((specifier) => specifier.local.name)
      : [],
  )
  const binding = existingStylex[0] ?? unique('stylex')
  const tokenBindings = new Map<string, string>()

  // Compound CSS values interpolate typed variables without changing their CSS spelling.
  const serialize = (value: StyleRule | string | number | null): string => {
    // Preserve scalar CSS values without treating them as rule objects.
    if (value === null || typeof value === 'number')
      return JSON.stringify(value)

    // Keep condition objects nested; only leaf values contain variable references.
    if (typeof value !== 'string')
      return `{${Object.entries(value)
        .map(([key, nested]) => `${JSON.stringify(key)}:${serialize(nested)}`)
        .join(',')}}`

    // Translate known theme variables while retaining fallback expressions and internal CSS variables.
    const pattern = /var\((--[\w-]+)\)/g
    const matches = [...value.matchAll(pattern)].filter((match) =>
      oracle.tokens.has(match[1]!),
    )
    if (!matches.length) return JSON.stringify(value)

    // Allocate each variable-group import only when a declaration references it.
    const reference = (key: string) => {
      const group = oracle.tokens.get(key)!
      let name = tokenBindings.get(group)
      if (!name) {
        name = unique(`theme${group[0]!.toUpperCase()}${group.slice(1)}`)
        tokenBindings.set(group, name)
      }
      return `${name}[${JSON.stringify(key)}]`
    }
    if (matches.length === 1 && matches[0]![0] === value)
      return reference(matches[0]![1]!)

    // Escape literal fragments independently from the inserted TypeScript expressions.
    let result = ''
    let offset = 0
    for (const match of matches) {
      result += value
        .slice(offset, match.index)
        .replaceAll('\\', '\\\\')
        .replaceAll('`', '\\`')
        .replaceAll('${', '\\${')
      result += '${' + reference(match[1]!) + '}'
      offset = match.index! + match[0].length
    }
    result += value
      .slice(offset)
      .replaceAll('\\', '\\\\')
      .replaceAll('`', '\\`')
      .replaceAll('${', '\\${')
    return '`' + result + '`'
  }

  // Resolve expression leaves in the current JSX scope and retain only consumed lexical bindings.
  const consumedBindings = new Set<string>()
  const expanding = new Set<string>()
  let resolveBinding: (name: string) => Syntax | undefined = () => undefined
  let isStyleInput: (name: string) => boolean = () => false
  let isUnbound: (name: string) => boolean = () => false
  let isClassHelper: (name: string) => boolean = () => false
  let resolveRecipe: (name: string) => Syntax | undefined = () => undefined
  const slice = (node: Syntax) => source.slice(node.start!, node.end!)

  // Preserve already-boolean conditions while excluding numeric falsy values from StyleX arrays.
  const truthy = (node: Syntax): string => {
    const boolean =
      node.type === 'BooleanLiteral' ||
      (node.type === 'UnaryExpression' && node.operator === '!') ||
      (node.type === 'BinaryExpression' &&
        [
          '===',
          '!==',
          '==',
          '!=',
          '<',
          '>',
          '<=',
          '>=',
          'in',
          'instanceof',
        ].includes(node.operator as string))
    return boolean ? `(${slice(node)})` : `!!(${slice(node)})`
  }

  // Keep an attribute manual when separate namespaces would replace another namespace's conditions.
  const checkComposition = (styles: string) => {
    // Recipes contribute their referenced namespaces just like direct class expressions.
    const references = new Set<string>()
    const collect = (value: string) => {
      for (const match of value.matchAll(
        new RegExp(`\\b${prefix}\\.([\\w$]+)`, 'g'),
      ))
        references.add(match[1]!)
    }
    collect(styles)
    for (const [name, recipe] of variantDefinitions)
      if (styles.includes(name + '(')) collect(recipe.source)

    // StyleX composition owns every condition of a property; Tailwind's cascade merges them separately.
    const seen = new Map<string, boolean>()
    const check = (rule: StyleRule, prefix = '') => {
      for (const [property, value] of Object.entries(rule)) {
        const key = prefix + property
        if (
          property.startsWith('::') &&
          value !== null &&
          typeof value === 'object'
        )
          check(value, key + '.')
        else {
          const conditional = value !== null && typeof value === 'object'
          if (seen.has(key) && (seen.get(key) || conditional))
            throw new Error(`conditional property composition: ${key}`)
          seen.set(key, conditional)
        }
      }
    }
    for (const entry of namespaces.values())
      if (references.has(entry.name)) check(propertyStyles(entry.style))
  }

  // Compile a whole class set in Tailwind order, including responsive and pseudo conditions.
  const literal = (value: string, normalize = false): string => {
    // The real class helper resolves conflicts before Tailwind sees a literal passed to cn.
    if (normalize) value = oracle.normalize(value)

    // Empty and repeated class sets need no additional style namespace.
    if (!value.trim()) return 'false'
    const existing = namespaces.get(value)
    if (existing) return `${prefix}.${existing.name}`

    // Resolve conflicts inside one literal before StyleX assigns its atomic priorities.
    const style: StyleRule = {}
    for (const candidate of oracle.order(value.trim().split(/\s+/))) {
      const translation = oracle.translate(candidate)
      if (translation.kind === 'manual')
        throw new Error(`${translation.reason}: ${candidate}`)
      mergeStyle(style, translation.style)
    }
    const name = `rule${namespaces.size + 1}`
    namespaces.set(value, { name, style })
    return `${prefix}.${name}`
  }

  // Preserve JavaScript branch evaluation while replacing only its class-string leaves.
  const expression = (node: Syntax, normalize = false): string => {
    switch (node.type) {
      case 'Identifier': {
        const name = node.name as string
        if (name === 'undefined' && isUnbound(name)) return 'undefined'
        if (options.componentProps && isStyleInput(name)) return slice(node)
        const initializer = resolveBinding(name)
        if (!initializer || expanding.has(name))
          throw new Error('dynamic class or forwarded className')
        expanding.add(name)
        try {
          const result = expression(initializer, normalize)
          consumedBindings.add(name)
          return result
        } finally {
          expanding.delete(name)
        }
      }
      case 'MemberExpression':
        if (
          options.componentProps &&
          !node.computed &&
          (node.property as Syntax).type === 'Identifier' &&
          (node.property as Syntax).name === 'sx'
        )
          return slice(node)
        throw new Error('dynamic class or forwarded className')
      case 'StringLiteral':
        return literal(node.value as string, normalize)
      case 'TemplateLiteral': {
        if ((node.expressions as unknown[]).length)
          throw new Error('dynamic template')
        return literal(
          (node.quasis as Array<{ value: { cooked: string } }>)[0]!.value
            .cooked,
          normalize,
        )
      }
      case 'NullLiteral':
        return 'null'
      case 'BooleanLiteral':
        return 'false'
      case 'LogicalExpression':
        if (node.operator !== '&&') throw new Error('fallback class expression')
        return `(${truthy(node.left as Syntax)} && ${expression(node.right as Syntax, normalize)})`
      case 'ConditionalExpression':
        return `(${slice(node.test as Syntax)} ? ${expression(node.consequent as Syntax, normalize)} : ${expression(node.alternate as Syntax, normalize)})`
      case 'ArrayExpression':
        return `[${(node.elements as Syntax[]).map((item) => (item ? expression(item, normalize) : 'null')).join(', ')}]`
      case 'ObjectExpression':
        return `[${(node.properties as Syntax[])
          .map((property) => {
            if (
              property.type !== 'ObjectProperty' ||
              property.computed ||
              (property.key as Syntax).type !== 'StringLiteral'
            )
              throw new Error('computed class object')
            return `(${truthy(property.value as Syntax)} && ${literal((property.key as Syntax).value as string, normalize)})`
          })
          .join(', ')}]`
      case 'CallExpression': {
        const callee = node.callee as Syntax
        if (allowVariants && callee.type === 'Identifier') {
          const initializer = resolveRecipe(callee.name as string)
          if (
            initializer?.type === 'CallExpression' &&
            (initializer.callee as Syntax).type === 'Identifier' &&
            variantHelpers.has((initializer.callee as Syntax).name as string)
          ) {
            const name = callee.name as string
            if (!variantDefinitions.has(name))
              variantDefinitions.set(name, {
                start: initializer.start!,
                end: initializer.end!,
                source: compileVariants(
                  initializer as CallExpression,
                  (classes) => literal(classes, normalize),
                  binding,
                ),
              })
            const args = (node.arguments as Syntax[]).map((argument) => {
              if (argument.type !== 'ObjectExpression')
                throw new Error('dynamic cva arguments')
              return `{${(argument.properties as Syntax[])
                .map((property) => {
                  if (property.type !== 'ObjectProperty' || property.computed)
                    throw new Error('dynamic cva arguments')
                  const key = property.key as Syntax
                  const name =
                    key.type === 'Identifier'
                      ? (key.name as string)
                      : (key.value as string)
                  return `${JSON.stringify(name)}:${['class', 'className'].includes(name) ? expression(property.value as Syntax, normalize) : slice(property.value as Syntax)}`
                })
                .join(',')}}`
            })
            return `${name}(${args.join(',')})`
          }
        }
        if (
          callee.type !== 'Identifier' ||
          !isClassHelper(callee.name as string)
        )
          throw new Error('computed class helper')
        // Compile adjacent literal arguments together so their CSS conditions keep their defaults.
        const parts: string[] = []
        let literals: string[] = []
        const flush = () => {
          if (literals.length) parts.push(literal(literals.join(' '), true))
          literals = []
        }
        for (const argument of node.arguments as Syntax[]) {
          if (argument.type === 'StringLiteral')
            literals.push(argument.value as string)
          else if (argument.type !== 'BooleanLiteral') {
            flush()
            parts.push(expression(argument, true))
          }
        }
        flush()
        return `[${parts.join(', ')}]`
      }
      default:
        throw new Error('dynamic class or forwarded className')
    }
  }

  // Third-party components keep their ordinary React props; intrinsic elements use the sx macro.
  traverse(ast, {
    JSXOpeningElement(path) {
      // Resolve immutable class constants through the actual lexical scope.
      const opening = path.node as unknown as Syntax
      resolveBinding = (name) => {
        const binding = path.scope.getBinding(name)
        if (!binding?.constant || !binding.path.isVariableDeclarator())
          return undefined
        return binding.path.node.init as Syntax | undefined
      }
      isUnbound = (name) => !path.scope.getBinding(name)

      // A renamed component input stays a StyleX value instead of becoming a class-string leaf.
      isStyleInput = (name) => {
        const binding = path.scope.getBinding(name)
        return (
          !!binding &&
          binding.kind === 'param' &&
          binding.path.isObjectPattern() &&
          binding.path.node.properties.some((property) => {
            if (
              property.type !== 'ObjectProperty' ||
              property.key.type !== 'Identifier' ||
              property.key.name !== 'sx'
            )
              return false
            const value =
              property.value.type === 'AssignmentPattern'
                ? property.value.left
                : property.value
            return value.type === 'Identifier' && value.name === name
          })
        )
      }

      // Imported helper names lose their compiler meaning when a local binding shadows them.
      isClassHelper = (name) =>
        classHelpers.has(name) &&
        !!path.scope.getBinding(name)?.path.isImportSpecifier()

      // Public or non-JSX recipe consumers require a coordinated hand pass.
      resolveRecipe = (name) => {
        // Follow the constructor in its own scope before changing any of its consumers.
        const binding = path.scope.getBinding(name)
        if (
          !binding?.constant ||
          !binding.path.isVariableDeclarator() ||
          binding.path.parentPath.parentPath?.isExportNamedDeclaration()
        )
          return undefined
        const initializer = binding.path.node.init
        if (
          initializer?.type !== 'CallExpression' ||
          initializer.callee.type !== 'Identifier'
        )
          return undefined
        const helper = binding.path.scope.getBinding(
          initializer.callee.name,
        )?.path
        if (
          !helper?.isImportSpecifier() ||
          helper.node.imported.type !== 'Identifier' ||
          helper.node.imported.name !== 'cva' ||
          !helper.parentPath.isImportDeclaration() ||
          helper.parentPath.node.source.value !== 'class-variance-authority'
        )
          return undefined
        if (
          binding.referencePaths.some(
            (reference) =>
              !reference.findParent(
                (parent) =>
                  (parent.isJSXAttribute() &&
                    parent.node.name.type === 'JSXIdentifier' &&
                    parent.node.name.name === 'className') ||
                  parent.isTSType(),
              ),
          )
        )
          return undefined
        return binding.path.node.init as Syntax | undefined
      }

      // Convert an attribute atomically so unsupported leaves retain their whole expression.
      const tag = opening.name as Syntax
      const native =
        tag.type === 'JSXIdentifier' && /^[a-z]/.test(tag.name as string)
      const owned = options.components?.has(
        tag.type === 'JSXIdentifier' ? (tag.name as string) : slice(tag),
      )
      if (options.onlyComponents && !owned) return
      for (const attribute of opening.attributes as Syntax[]) {
        // A sibling may have acquired sx after this caller's classes were compiled in an earlier pass.
        if (owned && attribute.type === 'JSXSpreadAttribute') {
          const argument = attribute.argument as Syntax
          const callee =
            argument.type === 'CallExpression'
              ? (argument.callee as Syntax)
              : undefined
          if (
            callee?.type === 'MemberExpression' &&
            !(callee.computed as boolean) &&
            (callee.property as Syntax).type === 'Identifier' &&
            (callee.property as Syntax).name === 'props' &&
            (callee.object as Syntax).type === 'Identifier'
          ) {
            const imported = path.scope.getBinding(
              (callee.object as Syntax).name as string,
            )?.path
            if (
              imported?.isImportNamespaceSpecifier() &&
              imported.parentPath.isImportDeclaration() &&
              imported.parentPath.node.source.value === '@stylexjs/stylex'
            ) {
              const args = argument.arguments as Syntax[]
              edits.push({
                start: attribute.start!,
                end: attribute.end!,
                text: `sx={${args.length === 1 && args[0]!.type !== 'SpreadElement' ? slice(args[0]!) : `[${args.map(slice).join(',')}]`}}`,
              })
              converted++
              continue
            }
          }
        }
        if (
          attribute.type !== 'JSXAttribute' ||
          (attribute.name as Syntax).name !== 'className'
        )
          continue
        const value = attribute.value as Syntax | null
        if (!value) continue
        const content =
          value.type === 'JSXExpressionContainer'
            ? (value.expression as Syntax)
            : value
        const checkpoint = new Map(namespaces)
        try {
          const styles = expression(content)
          checkComposition(styles)
          converted++
          edits.push({
            start: attribute.start!,
            end: attribute.end!,
            text:
              native || owned
                ? `sx={${styles}}`
                : `{...${binding}.props(${styles})}`,
          })
        } catch (error) {
          namespaces.clear()
          for (const [key, entry] of checkpoint) namespaces.set(key, entry)
          const reason = (error as Error).message
          remaining.push({
            line: attribute.loc!.start.line,
            reason,
            source: slice(attribute),
          })
          if (!slice(attribute).includes('TODO(stylex)')) {
            edits.push({
              start: attribute.start!,
              end: attribute.end!,
              text: `className={/* TODO(stylex): ${reason.replaceAll('*/', '* /')} */ ${slice(content)}}`,
            })
          }
        }
      }
    },
  })

  // A recipe changes only when every attribute in this module is ready for StyleX composition.
  if (variantDefinitions.size && remaining.length)
    return transform(source, filename, oracle, options, false)
  if (variantDefinitions.size) {
    for (const recipe of variantDefinitions.values())
      edits.push({ start: recipe.start, end: recipe.end, text: recipe.source })
  }

  // Apply edits from the end; a second run sees no className at these sites.
  if (!edits.length) return { source, converted: 0, remaining }
  if (namespaces.size) {
    const definitions = Object.fromEntries(
      [...namespaces.values()].map(({ name, style }) => [
        name,
        propertyStyles(style),
      ]),
    )
    const styles = `\nconst ${prefix} = ${binding}.create(${serialize(definitions)})\n\n`
    let imports = existingStylex.length
      ? ''
      : `import * as ${binding} from '@stylexjs/stylex'\n`
    if (tokenBindings.size) {
      let tokenPath = relative(
        dirname(resolve(filename)),
        resolve('web/style/tokens.stylex.js'),
      ).replaceAll('\\', '/')
      if (!tokenPath.startsWith('.')) tokenPath = './' + tokenPath
      imports += `import {${[...tokenBindings].map(([group, name]) => `${group} as ${name}`).join(',')}} from ${JSON.stringify(tokenPath)}\n`
    }
    const firstBody = ast.program.body.find(
      (node) => node.type !== 'ImportDeclaration',
    )
    const insertion =
      firstBody?.leadingComments?.[0]?.start ??
      firstBody?.start ??
      source.length
    edits.push({
      start: insertion,
      end: insertion,
      text: imports + styles,
    })
  }
  for (const edit of edits.sort((left, right) => right.start - left.start))
    source = source.slice(0, edit.start) + edit.text + source.slice(edit.end)

  // Drop only consumed class constants and the imported helpers whose references disappeared.
  const updated = parseSync(source, {
    filename,
    configFile: false,
    babelrc: false,
    parserOpts: {
      plugins: /\.[cm]?ts$/.test(filename)
        ? ['typescript']
        : ['typescript', 'jsx'],
    },
  })!
  const removals: Edit[] = []
  traverse(updated, {
    VariableDeclarator(path) {
      // Keep exports and any binding still referenced by an unconverted expression.
      if (
        path.node.id.type !== 'Identifier' ||
        !consumedBindings.has(path.node.id.name)
      )
        return
      const binding = path.scope.getBinding(path.node.id.name)
      const declaration = path.parentPath
      if (
        binding?.referenced ||
        !declaration.isVariableDeclaration() ||
        declaration.node.declarations.length !== 1 ||
        declaration.parentPath.isExportNamedDeclaration()
      )
        return
      removals.push({
        start: declaration.node.start!,
        end: declaration.node.end!,
        text: '',
      })
    },
    ImportDeclaration(path) {
      const specifier = path.node.specifiers[0]
      if (
        path.node.specifiers.length !== 1 ||
        !specifier ||
        (!classHelpers.has(specifier.local.name) &&
          !variantHelpers.has(specifier.local.name)) ||
        path.scope.getBinding(specifier.local.name)?.referenced
      )
        return
      removals.push({ start: path.node.start!, end: path.node.end!, text: '' })
    },
  })
  for (const edit of removals.sort((left, right) => right.start - left.start))
    source = source.slice(0, edit.start) + source.slice(edit.end)
  return { source, converted, remaining }
}
