import type { AssignmentPattern, Node, ObjectPattern } from '@babel/types'

/** isDefaultObjectParameter identifies destructured props with an empty object default. */
export function isDefaultObjectParameter(
  node: Node | undefined,
): node is AssignmentPattern & { left: ObjectPattern } {
  return (
    node?.type === 'AssignmentPattern' &&
    node.left.type === 'ObjectPattern' &&
    node.right.type === 'ObjectExpression' &&
    node.right.properties.length === 0
  )
}
