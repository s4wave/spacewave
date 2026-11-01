import type { CallExpression, Node, ObjectExpression } from '@babel/types'

// entries requires a static recipe so conversion never evaluates application code.
function entries(node: Node | null | undefined): Map<string, Node> {
  if (!node || node.type !== 'ObjectExpression')
    throw new Error('dynamic cva configuration')
  const result = new Map<string, Node>()
  for (const property of (node as ObjectExpression).properties) {
    if (property.type !== 'ObjectProperty' || property.computed)
      throw new Error('computed cva configuration')
    const key = property.key
    if (key.type !== 'Identifier' && key.type !== 'StringLiteral')
      throw new Error('computed cva key')
    result.set(key.type === 'Identifier' ? key.name : key.value, property.value)
  }
  return result
}

// scalar retains cva's null, boolean and string selection values.
function scalar(node: Node): string {
  if (node.type === 'StringLiteral' || node.type === 'BooleanLiteral')
    return JSON.stringify(node.value)
  if (node.type === 'NullLiteral') return 'null'
  throw new Error('dynamic cva default')
}

/** compileVariants replaces a static cva recipe with typed StyleX composition. */
export function compileVariants(
  node: CallExpression,
  literal: (classes: string) => string,
  binding: string,
): string {
  // Read static class fragments without evaluating the recipe or its callers.
  const classes = (node: Node | undefined | null): string => {
    if (!node || node.type === 'NullLiteral') return ''
    if (node.type === 'StringLiteral') return node.value
    if (node.type === 'ArrayExpression')
      return node.elements.map(classes).join(' ')
    throw new Error('dynamic cva classes')
  }
  const base = classes(node.arguments[0])
  const config = node.arguments[1]
    ? entries(node.arguments[1])
    : new Map<string, Node>()
  for (const key of config.keys())
    if (!['variants', 'defaultVariants', 'compoundVariants'].includes(key))
      throw new Error(`unknown cva option ${key}`)

  // Resolve each typed selection once, preserving the difference between null and the default.
  const variants = config.has('variants')
    ? entries(config.get('variants'))
    : new Map<string, Node>()
  const defaults = config.has('defaultVariants')
    ? entries(config.get('defaultVariants'))
    : new Map<string, Node>()
  const types: string[] = []
  const declarations: string[] = []

  // Declare one runtime selection per axis and derive its public input type.
  const axes = [...variants].map(([name, options], index) => {
    // Resolve the axis before applying its optional default.
    const choices = entries(options)
    const variable = `variant${index}`
    const fallback = defaults.has(name)
      ? scalar(defaults.get(name)!)
      : 'undefined'
    declarations.push(
      `// Resolve ${JSON.stringify(name)} once, retaining explicit null instead of its default.\nconst ${variable}Input=props?.[${JSON.stringify(name)}];const ${variable}=${variable}Input===undefined?${fallback}:${variable}Input;\n`,
    )

    // Keep the caller contract limited to the recipe's declared option values.
    const choiceTypes = [...choices.keys()].map((key) =>
      ['true', 'false'].includes(key) ? key : JSON.stringify(key),
    )
    types.push(`${JSON.stringify(name)}?:${choiceTypes.join('|')}|null`)
    return { name, choices, variable }
  })

  // Merge whole recipe outcomes through Tailwind before StyleX owns their conditional properties.
  const compounds = config.get('compoundVariants')
  if (compounds && compounds.type !== 'ArrayExpression')
    throw new Error('dynamic cva compounds')
  const rules = (compounds?.elements ?? []).map((compound) => entries(compound))
  const selected = new Map<string, string>()
  const outcome = (index: number, fragments: string[]): string => {
    // Each leaf is one class set, so a variant cannot erase a base hover or breakpoint rule.
    if (index === axes.length) {
      const matches: string[] = []
      for (const fields of rules) {
        let applies = true
        for (const [name, value] of fields) {
          if (name === 'class' || name === 'className') continue
          if (!selected.has(name))
            throw new Error(`unknown cva compound variant ${name}`)
          const expected =
            value.type === 'ArrayExpression'
              ? value.elements.map((item) => scalar(item!))
              : [scalar(value)]
          applies &&= expected.includes(selected.get(name)!)
        }
        if (applies)
          matches.push(
            classes(fields.get('class')),
            classes(fields.get('className')),
          )
      }
      return literal([base, ...fragments, ...matches].join(' '))
    }

    // Null disables an axis; an omitted axis without a default also contributes no classes.
    const axis = axes[index]!
    selected.set(axis.name, 'undefined')
    let result = outcome(index + 1, fragments)
    selected.set(axis.name, 'null')
    const absent = outcome(index + 1, fragments)
    if (absent !== result)
      result = `(${axis.variable}===null?${absent}:${result})`
    for (const [key, value] of [...axis.choices].reverse()) {
      const choice = ['true', 'false'].includes(key) ? key : JSON.stringify(key)
      selected.set(axis.name, choice)
      result = `(${axis.variable}===${choice}?${outcome(index + 1, [...fragments, classes(value)])}:${result})`
    }
    return result
  }
  const compiled = outcome(0, [])
  types.push(
    `class?:${binding}.StyleXStyles`,
    `className?:${binding}.StyleXStyles`,
  )
  return `(props?:{${types.join(';')}})=>{\n${declarations.join('\n')}\n// Select one fully compiled recipe outcome, then compose caller styles.\nreturn [${compiled},props?.class,props?.className] as const\n}`
}
