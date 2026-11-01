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
  // Preserve base class arrays through the same utility compiler as ordinary attributes.
  const classes = (node: Node | undefined | null): string => {
    if (!node || node.type === 'NullLiteral') return 'false'
    if (node.type === 'StringLiteral') return literal(node.value)
    if (node.type === 'ArrayExpression')
      return `[${node.elements.map(classes).join(',')}]`
    throw new Error('dynamic cva classes')
  }
  const base = classes(node.arguments[0])
  const config = node.arguments[1]
    ? entries(node.arguments[1])
    : new Map<string, Node>()
  for (const key of config.keys())
    if (!['variants', 'defaultVariants', 'compoundVariants'].includes(key))
      throw new Error(`unknown cva option ${key}`)

  // Read each option once, applying its default only to undefined and preserving explicit null.
  const variants = config.has('variants')
    ? entries(config.get('variants'))
    : new Map<string, Node>()
  const defaults = config.has('defaultVariants')
    ? entries(config.get('defaultVariants'))
    : new Map<string, Node>()
  const types: string[] = []
  const declarations: string[] = []
  const selected: string[] = [base]
  const values = new Map<string, string>()
  for (const [name, options] of variants) {
    const choices = entries(options)
    const variable = `variant${values.size}`
    const raw = `${variable}Input`
    values.set(name, variable)
    const fallback = defaults.has(name)
      ? scalar(defaults.get(name)!)
      : 'undefined'
    declarations.push(
      `const ${raw}=props?.[${JSON.stringify(name)}];const ${variable}=${raw}===undefined?${fallback}:${raw};`,
    )
    const choiceTypes = [...choices.keys()].map((key) =>
      ['true', 'false'].includes(key) ? key : JSON.stringify(key),
    )
    types.push(`${JSON.stringify(name)}?:${choiceTypes.join('|')}|null`)
    for (const [key, value] of choices) {
      const choice = ['true', 'false'].includes(key) ? key : JSON.stringify(key)
      selected.push(`(${variable}===${choice}&&${classes(value)})`)
    }
  }

  // Compound choices use the resolved defaults, as cva does before appending caller styles.
  const compounds = config.get('compoundVariants')
  if (compounds && compounds.type !== 'ArrayExpression')
    throw new Error('dynamic cva compounds')
  for (const compound of compounds?.elements ?? []) {
    const fields = entries(compound)
    const conditions: string[] = []
    for (const [name, value] of fields) {
      if (name === 'class' || name === 'className') continue
      const variable = values.get(name)
      if (!variable) throw new Error(`unknown cva compound variant ${name}`)
      conditions.push(
        value.type === 'ArrayExpression'
          ? `(${value.elements.map((item) => `${variable}===${scalar(item!)}`).join('||') || 'false'})`
          : `${variable}===${scalar(value)}`,
      )
    }
    const styles = `[${classes(fields.get('class'))},${classes(fields.get('className'))}]`
    selected.push(
      `(${conditions.length ? conditions.join('&&') : 'true'}&&${styles})`,
    )
  }
  types.push(
    `class?:${binding}.StyleXStyles`,
    `className?:${binding}.StyleXStyles`,
  )
  selected.push('props?.class', 'props?.className')
  return `(props?:{${types.join(';')}})=>{${declarations.join('')}return [${selected.join(',')}] as const}`
}
