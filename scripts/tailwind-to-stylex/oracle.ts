import { readFile } from 'node:fs/promises'
import { dirname } from 'node:path'
import {
  __unstable__loadDesignSystem,
  compile,
  optimize,
} from '@tailwindcss/node'
import postcss, { type Container, type Declaration } from 'postcss'
import { parseSync } from '@babel/core'
import { join } from 'node:path'
import { cn } from 'cnfast'
import { expandAnimation } from './animation.js'
import { expandBackground } from './background.js'
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

// conditionTarget gives each state its own StyleX priority instead of hiding it in one selector.
function conditionTarget(style: StyleRule, condition: string): StyleRule {
  const parts = condition.match(/:{1,2}[\w-]+(?:\([^)]*\))?|\[[^\]]+\]/g)!
  let target = style
  for (const part of parts) {
    const key = part.startsWith('[') ? `:is(${part})` : part
    target = (target[key] ??= {}) as StyleRule
  }
  return target
}

// readRule accepts only selectors that style the element carrying the class.
function readRule(container: Container, selector: string): StyleRule {
  // Preserve each CSS condition around the declarations it actually controls.
  const style: StyleRule = {}
  for (const node of container.nodes ?? []) {
    if (node.type === 'decl') {
      const declaration = node as Declaration
      if (declaration.important) throw new Error('important declaration')
      // Keep the forced-color outline's style in the same property as its normal default.
      if (declaration.prop === 'background') {
        // Custom utilities need the same longhand expansion as component recipes.
        const expanded = postcss.parse(expandBackground(declaration.value))
        expanded.walkDecls((value) => {
          const repeated = value.value.split(',').map((part) => part.trim())
          style[propertyName(value.prop)] = repeated.every(
            (part) => part === repeated[0],
          )
            ? repeated[0]!
            : value.value
        })
      } else if (declaration.prop === 'outline') {
        if (declaration.value !== '2px solid #0000')
          throw new Error('outline shorthand')
        Object.assign(style, {
          outlineWidth: '2px',
          outlineStyle: 'solid',
          outlineColor: '#0000',
        })
      } else {
        // CSS integer properties need numeric values for StyleX's public style type.
        const negativeInteger =
          declaration.prop === 'z-index'
            ? declaration.value.match(/^calc\((\d+) \* -1\)$/)
            : null
        const pixelPerspective =
          declaration.prop === 'perspective'
            ? declaration.value.match(/^(-?[\d.]+)px$/)
            : null
        style[propertyName(declaration.prop)] = negativeInteger
          ? -Number(negativeInteger[1])
          : pixelPerspective
            ? Number(pixelPerspective[1])
            : declaration.value
      }
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
        .replace(/(?<!:):(before|after|first-letter|first-line)$/, '::$1')
      if (!condition) Object.assign(style, readRule(node, selector))
      else if (
        /^:{1,2}(?:[\w-]+(?:\([^)]*\))?)(?::[\w-]+(?:\([^)]*\))?)*$/.test(
          condition,
        ) ||
        /^(?::[\w-]+(?:\([^)]*\))?|\[[^\]]+\])+$/.test(condition)
      )
        Object.assign(
          conditionTarget(style, condition),
          readRule(node, selector),
        )
      else {
        const pseudo = condition.match(
          /^(.*)(::(?:before|after|first-letter|first-line))$/,
        )
        const relation = relationCondition(pseudo?.[1] ?? condition)
        if (!relation)
          throw new Error('a descendant, group, peer or compound selector')
        const target = pseudo
          ? ((style[pseudo[2]!] ??= {}) as StyleRule)
          : style
        target[relation] = readRule(node, selector)
      }
    }
  }
  return style
}

// readComponentClasses accepts one plain rule below the utility layer and rejects related selectors.
function readComponentClasses(
  css: string,
  tokens: ReadonlyMap<string, string>,
): Map<string, { style: StyleRule; order: number }> {
  // Any second selector or conditional occurrence keeps the whole class manual.
  const rules = new Map<string, { style: StyleRule; order: number }>()
  const seen = new Set<string>()
  const blocked = new Set<string>()
  postcss.parse(css).walkRules((rule) => {
    // Related or repeated selectors keep every part of their class in the hand pass.
    const classes = [...rule.selector.matchAll(/\.([\w-]+)/g)].map(
      (match) => match[1]!,
    )
    for (const name of classes) {
      if (seen.has(name)) blocked.add(name)
      seen.add(name)
    }
    const plain = rule.selector.match(/^\.([\w-]+)$/)?.[1]
    if (!plain) {
      for (const name of classes) blocked.add(name)
      return
    }

    // Component helpers are lower than every utility; other layers need their own cascade decision.
    let parent = rule.parent
    let component = false
    let supported = true
    while (parent && parent.type !== 'root') {
      if (
        parent.type !== 'atrule' ||
        parent.name !== 'layer' ||
        parent.params !== 'components'
      )
        supported = false
      else component = true
      parent = parent.parent
    }
    if (
      !component ||
      !supported ||
      rule.nodes.some(
        (node) =>
          node.type !== 'decl' || node.important || node.prop === 'animation',
      )
    ) {
      blocked.add(plain)
      return
    }

    // Native shorthand expansion preserves resets; unresolved variable shorthands remain manual.
    try {
      const expanded = rule.clone()
      expanded.walkDecls('background', (declaration) => {
        // Expand parsed resets while retaining runtime references to known color tokens.
        const variable = declaration.value.match(/^var\((--[\w-]+)\)$/)?.[1]
        const css = postcss.parse(
          expandBackground(
            declaration.value,
            !!variable && tokens.get(variable) === 'colors',
          ),
        )
        const values: Declaration[] = []
        css.walkDecls((value) => {
          values.push(value.clone())
        })
        declaration.replaceWith(...values)
      })
      rules.set(plain, {
        style: readRule(expanded, rule.selector),
        order: rules.size,
      })
    } catch {
      blocked.add(plain)
    }
  })

  // A declaration shared with a descendant, pseudo or responsive rule cannot leave that rule behind.
  for (const name of blocked) rules.delete(name)
  return rules
}

// readFontAliases keeps only single unlayered typography aliases with no other selectors.
function readFontAliases(
  css: string,
  names: ReadonlySet<string>,
): Map<string, StyleRule> {
  // Count every selector occurrence before accepting an alias's complete declarations.
  const counts = new Map<string, number>()
  const candidates = new Map<string, StyleRule>()
  postcss.parse(css).walkRules((rule) => {
    for (const selector of rule.selectors) {
      const name = selector.match(/^\.([\w-]+)(?:$|[:[.])/)?.[1]
      if (!name || !names.has(name)) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
      if (
        selector !== '.' + name ||
        rule.parent?.type !== 'root' ||
        !rule.nodes.length ||
        !rule.nodes.every(
          (node) =>
            node.type === 'decl' &&
            !node.important &&
            ['font-size', 'line-height', 'letter-spacing'].includes(node.prop),
        )
      )
        continue
      candidates.set(name, readRule(rule, selector))
    }
  })

  // A second rule would add a cascade contract this typography-only path does not represent.
  for (const name of candidates.keys())
    if (counts.get(name) !== 1) candidates.delete(name)
  return candidates
}

/** createOracle translates utilities using the repository's actual Tailwind theme. */
export async function createOracle(theme: string) {
  // Load imports, custom utilities and variants through Tailwind's own Node resolver.
  const themeSource = await readFile(theme, 'utf8')
  const design = await __unstable__loadDesignSystem(themeSource, {
    base: dirname(theme),
  })

  // Compile application helper declarations once, including imports and @apply expansion.
  const application = await compile(themeSource, {
    base: dirname(theme),
    onDependency() {},
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

  // The token inventory distinguishes runtime color variables from unresolved background recipes.
  const applicationCss = application.build([])
  const componentClasses = readComponentClasses(applicationCss, tokens)
  const fontAliases = readFontAliases(applicationCss, globalAliases)

  // Cache utility translations and their required property registrations for this one conversion.
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
    else if (globalAliases.has(candidate) && !fontAliases.has(candidate))
      result = {
        kind: 'manual',
        reason: 'global class adds declarations outside the utility compiler',
      }
    else if (!css)
      result = fontAliases.has(candidate)
        ? { kind: 'style', style: {} }
        : componentClasses.has(candidate)
          ? { kind: 'style', style: componentClasses.get(candidate)!.style }
          : { kind: 'manual', reason: 'global or unknown class' }
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
            selector =
              rule.selector.match(/^\.(?:\\.|[\w\u0080-\uFFFF-])+/)?.[0] ?? ''
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
          ? (componentClasses.get(left[0])?.order ?? 0) -
            (componentClasses.get(right[0])?.order ?? 0)
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
    fontAliases,
    order,
    globalRules,
    tokens,
    normalize: (classes: string) => cn(classes),
  }
}
