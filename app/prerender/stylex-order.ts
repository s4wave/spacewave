import { readFileSync } from 'node:fs'
import postcss, { type AtRule, type Container } from 'postcss'

import {
  stylexLayerOrder,
  stylexPriority,
} from '../../bldr/web/bldr/stylex-priorities.js'

// collectPriorities handles nested layer names produced by CSS minification.
function collectPriorities(
  container: Container,
  priorities: Set<string>,
  prefix = '',
): void {
  container.each((node) => {
    // Keep each enclosing layer prefix while visiting nested media and layer blocks.
    if (node.type !== 'atrule') return
    const rule = node as AtRule
    let nested = prefix
    if (rule.name === 'layer') {
      for (const name of rule.params.split(',')) {
        const qualified = prefix + name.trim()
        const priority = stylexPriority(qualified)
        if (priority !== undefined) priorities.add(qualified)
      }
      if (rule.nodes) nested += `${rule.params.trim()}.`
    }
    if (rule.nodes) collectPriorities(rule, priorities, nested)
  })
}

/** readStylexOrder collects one initial order while retaining at most one CSS syntax tree. */
export function readStylexOrder(files: Iterable<string>): string {
  const priorities = new Set<string>()
  for (const file of files)
    collectPriorities(
      postcss.parse(readFileSync(file, 'utf8'), { from: file }),
      priorities,
    )
  return stylexLayerOrder(priorities)
}
