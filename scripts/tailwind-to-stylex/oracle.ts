import { readFile } from 'node:fs/promises'
import { dirname } from 'node:path'
import { __unstable__loadDesignSystem, optimize } from '@tailwindcss/node'
import postcss, { type Container, type Declaration } from 'postcss'
import { parseSync } from '@babel/core'
import { join } from 'node:path'
import { cn } from 'cnfast'
import { expandAnimation } from './animation.js'
import { relationMarkers, relationCondition } from './relations.js'

/** StyleRule is a StyleX declaration object with nested conditions. */
export interface StyleRule {
  [property: string]: string | number | null | StyleRule
}

/** Translation retains an unsupported class for the component's hand pass. */
export type Translation =
  | { kind: 'style'; style: StyleRule }
  | { kind: 'marker'; marker: string }
  | { kind: 'manual'; reason: string }

// propertyName preserves custom properties and converts CSS names to StyleX names.
const propertyName = (name: string) =>
  name.startsWith('--')
    ? name
    : name.replace(/-([a-z])/g, (_, letter: string) => letter.toUpperCase())

// readRule accepts only selectors that style the element carrying the class.
function readRule(container: Container, selector: string): StyleRule {
  // Preserve each CSS condition around the declarations it actually controls.
  const style: StyleRule = {}
  for (const node of container.nodes ?? []) {
    if (node.type === 'decl') {
      const declaration = node as Declaration
      if (declaration.important) throw new Error('important declaration')
      style[propertyName(declaration.prop)] = declaration.value
    } else if (node.type === 'atrule') {
      if (!['media', 'container', 'supports'].includes(node.name))
        throw new Error(`@${node.name} rule`)
      style[`@${node.name} ${node.params}`] = readRule(node, selector)
    } else if (node.type === 'rule') {
      if (!node.selector.startsWith(selector))
        throw new Error('a selector outside the styled element')
      // CSS optimization prints legacy pseudo-elements with one colon; keep them as separate targets.
      const condition = node.selector
        .slice(selector.length)
        .replace(/^:(before|after|first-letter|first-line)(?=$|:)/, '::$1')
      if (!condition) Object.assign(style, readRule(node, selector))
      else if (
        /^:{1,2}(?:[\w-]+(?:\([^)]*\))?)(?::[\w-]+(?:\([^)]*\))?)*$/.test(
          condition,
        )
      )
        style[condition] = readRule(node, selector)
      else if (/^(?::[\w-]+(?:\([^)]*\))?|\[[^\]]+\])+$/.test(condition))
        style[`:is(${condition})`] = readRule(node, selector)
      else {
        const relation = relationCondition(condition)
        if (!relation)
          throw new Error('a descendant, group, peer or compound selector')
        style[relation] = readRule(node, selector)
      }
    }
  }
  return style
}

/** createOracle translates utilities using the repository's actual Tailwind theme. */
export async function createOracle(theme: string) {
  // Load imports, custom utilities and variants through Tailwind's own Node resolver.
  const themeSource = await readFile(theme, 'utf8')
  const design = await __unstable__loadDesignSystem(themeSource, {
    base: dirname(theme),
  })

  // Global aliases can add typography to a utility name; retain those whole for their hand pass.
  const globalAliases = new Set<string>()
  postcss.parse(themeSource).walkRules((rule) => {
    if (
      !rule.nodes.some(
        (node) => node.type === 'atrule' && node.name === 'apply',
      )
    )
      return
    for (const selector of rule.selectors) {
      const name = selector.match(/^\.([\w-]+)(?:$|[:[.])/)?.[1]
      if (name) globalAliases.add(name)
    }
  })

  // Typed theme references retain the public literal variable names during extraction.
  const tokens = new Map<string, string>()
  const tokenSource = parseSync(
    await readFile(join(dirname(theme), 'tokens.stylex.ts'), 'utf8'),
    {
      configFile: false,
      babelrc: false,
      parserOpts: { plugins: ['typescript'] },
    },
  )!
  for (const statement of tokenSource.program.body) {
    if (
      statement.type !== 'ExportNamedDeclaration' ||
      statement.declaration?.type !== 'VariableDeclaration'
    )
      continue
    for (const declaration of statement.declaration.declarations) {
      if (
        declaration.id.type !== 'Identifier' ||
        declaration.init?.type !== 'CallExpression'
      )
        continue
      const values = declaration.init.arguments[0]
      if (values?.type !== 'ObjectExpression') continue
      for (const property of values.properties) {
        if (
          property.type === 'ObjectProperty' &&
          property.key.type === 'StringLiteral' &&
          property.key.value.startsWith('--')
        )
          tokens.set(property.key.value, declaration.id.name)
      }
    }
  }
  const cache = new Map<string, Translation>()
  const globalRules = new Set<string>()

  // Each candidate is compiled once; unsupported selectors stay explicit for a hand pass.
  const translate = (candidate: string): Translation => {
    // Reuse this class’s compiler result across every source file.
    const cached = cache.get(candidate)
    if (cached) return cached

    // Keep the compiler's exact declarations and conditions for this candidate.
    const css = design.candidatesToCss([candidate])[0]
    let result: Translation
    if (relationMarkers[candidate])
      result = { kind: 'marker', marker: candidate }
    else if (globalAliases.has(candidate))
      result = {
        kind: 'manual',
        reason: 'global class adds declarations outside the utility compiler',
      }
    else if (!css)
      result = { kind: 'manual', reason: 'global or unknown class' }
    else {
      try {
        const declarations = postcss.parse(css)
        declarations.walkAtRules('property', (rule) => {
          globalRules.add(rule.toString())
          rule.remove()
        })
        // Tailwind's Vite plugin optimizes declarations before native CSS processing.
        const root = postcss.parse(
          optimize(declarations.toString(), { file: theme }).code,
        )
        // StyleX requires animation longhands; resolve fixed theme recipes before parsing them.
        root.walkDecls('animation', (declaration) => {
          const value = declaration.value.replace(
            /var\((--[\w-]+)\)/g,
            (reference, name: string) => design.theme.get([name]) ?? reference,
          )
          const longhands: Declaration[] = []
          postcss.parse(expandAnimation(value)).walkDecls((value) => {
            longhands.push(value)
          })
          declaration.replaceWith(...longhands)
        })
        let selector = ''
        root.walkRules((rule) => {
          if (!selector)
            selector = rule.selector.match(/^\.(?:\\.|[\w-])+/)?.[0] ?? ''
        })
        if (!selector)
          throw new Error('a descendant, group, peer or compound selector')
        const style = readRule(root, selector)
        if (candidate === 'font-mono')
          Object.assign(style, {
            WebkitFontSmoothing: 'subpixel-antialiased',
            MozOsxFontSmoothing: 'auto',
            textRendering: 'auto',
            textShadow: '0 0 0.25px currentColor',
          })
        result = { kind: 'style', style }
      } catch (error) {
        result = { kind: 'manual', reason: (error as Error).message }
      }
    }
    cache.set(candidate, result)
    return result
  }

  // Tailwind's canonical order, rather than source-string order, resolves conflicting utilities.
  const order = (candidates: string[]) =>
    design
      .getClassOrder(candidates)
      .sort((left, right) =>
        left[1] === right[1]
          ? 0
          : left[1] === null
            ? -1
            : right[1] === null
              ? 1
              : left[1] < right[1]
                ? -1
                : 1,
      )
      .map(([candidate]) => candidate)
  return {
    translate,
    order,
    globalRules,
    tokens,
    normalize: (classes: string) => cn(classes),
  }
}
