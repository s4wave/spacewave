import type { Node } from '@babel/types'

type Fragment = { kind: 'class' | 'native'; value: string }
type Continuation = (fragments: Fragment[]) => string

/** FiniteClassScope resolves class inputs without executing application code. */
export interface FiniteClassScope {
  slice: (node: Node) => string
  literal: (classes: string) => string
  normalize: (classes: string) => string
  resolveBinding: (name: string) => Node | undefined
  isUnbound: (name: string) => boolean
  isStyleInput: (name: string) => boolean
  isClassHelper: (name: string) => boolean
  consumed: (name: string) => void
}

/** compileFiniteClasses emits complete outcomes when atomic properties share conditions. */
export function compileFiniteClasses(
  input: Node,
  scope: FiniteClassScope,
): string {
  // Preserve native overrides after the complete class outcome they replace.
  const render: Continuation = (fragments) => {
    const classes: string[] = [],
      native: string[] = []
    for (const fragment of fragments) {
      if (fragment.kind === 'native') native.push(fragment.value)
      else if (fragment.value.trim()) {
        if (native.length)
          throw Error('class defaults after a native style input')
        classes.push(fragment.value)
      }
    }
    const base = scope.literal(classes.join(' '))
    return native.length ? `[${base},${native.join(',')}]` : base
  }

  // Normalize each actual cn call before its result participates in an outer call.
  const normalize = (fragments: Fragment[]): Fragment[] => {
    // Keep each class run on its original side of native overrides.
    const result: Fragment[] = []
    let classes: string[] = []
    const flush = () => {
      if (classes.length)
        result.push({
          kind: 'class',
          value: scope.normalize(classes.join(' ')),
        })
      classes = []
    }
    for (const fragment of fragments) {
      if (fragment.kind === 'class') classes.push(fragment.value)
      else {
        flush()
        result.push(fragment)
      }
    }
    flush()
    return result
  }

  // A conditional already coerces its test, so an explicit double cast is redundant there.
  const predicate = (node: Node) =>
    scope.slice(
      node.type === 'UnaryExpression' &&
        node.operator === '!' &&
        node.argument.type === 'UnaryExpression' &&
        node.argument.operator === '!'
        ? node.argument.argument
        : node,
    )

  // Traverse one runtime path at a time, keeping each condition's evaluation order.
  function sequence(
    nodes: Node[],
    done: Continuation,
    stack: ReadonlySet<string>,
    index = 0,
    fragments: Fragment[] = [],
  ): string {
    if (index === nodes.length) return done(fragments)
    return walk(
      nodes[index]!,
      (part) =>
        sequence(nodes, done, stack, index + 1, [...fragments, ...part]),
      stack,
    )
  }
  function walk(
    node: Node,
    done: Continuation,
    stack: ReadonlySet<string>,
  ): string {
    // Literal and native leaves retain their separate composition contracts.
    if (node.type === 'StringLiteral')
      return done([{ kind: 'class', value: node.value }])
    if (node.type === 'NullLiteral' || node.type === 'BooleanLiteral')
      return done([])
    if (node.type === 'Identifier') {
      if (node.name === 'undefined' && scope.isUnbound(node.name))
        return done([])
      if (scope.isStyleInput(node.name))
        return done([{ kind: 'native', value: node.name }])
      const binding = scope.resolveBinding(node.name)
      if (!binding || stack.has(node.name))
        throw Error('dynamic class or forwarded className')
      const result = walk(binding, done, new Set([...stack, node.name]))
      scope.consumed(node.name)
      return result
    }

    // Branches remain lazy and evaluate the original test exactly once per path.
    if (node.type === 'ConditionalExpression')
      return `(${predicate(node.test)}?${walk(node.consequent, done, stack)}:${walk(node.alternate, done, stack)})`
    if (node.type === 'LogicalExpression' && node.operator === '&&')
      return `(${predicate(node.left)}?${walk(node.right, done, stack)}:${done([])})`
    if (node.type === 'ArrayExpression') {
      if (node.elements.some((n) => n?.type === 'SpreadElement'))
        throw Error('spread class array')
      return sequence(
        node.elements.filter((n): n is Exclude<typeof n, null> => n !== null),
        done,
        stack,
      )
    }

    // Only the imported class helper may combine class fragments.
    if (
      node.type === 'CallExpression' &&
      node.callee.type === 'Identifier' &&
      scope.isClassHelper(node.callee.name)
    ) {
      if (
        node.arguments.some(
          (n) => n.type === 'SpreadElement' || n.type === 'ArgumentPlaceholder',
        )
      )
        throw Error('spread class helper')
      return sequence(
        node.arguments as Node[],
        (part) => done(normalize(part)),
        stack,
      )
    }
    throw Error('non-finite class expression: ' + node.type)
  }
  return walk(input, render, new Set())
}
