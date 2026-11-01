/** stylexLayersBefore keeps the reset and transitional Tailwind layers below StyleX. */
export const stylexLayersBefore = [
  'reset',
  'theme',
  'base',
  'components',
  'utilities',
] as const

/** stylexPriority reads a stable compiler priority from a qualified layer name. */
export function stylexPriority(name: string): number | undefined {
  const match = /^stylex\.priority(\d+(?:_\d+)?)$/.exec(name)
  return match ? Number(match[1].replace('_', '.')) : undefined
}

/** stylexLayerOrder declares one complete priority order before stylesheet rules. */
export function stylexLayerOrder(priorities: Iterable<number>): string {
  const names = [...new Set(priorities)]
    .sort((left, right) => left - right)
    .map((priority) => `stylex.priority${String(priority).replace('.', '_')}`)
  return names.length
    ? `@layer ${[...stylexLayersBefore, ...names].join(',')};`
    : ''
}
