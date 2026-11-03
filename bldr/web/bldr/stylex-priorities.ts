/** stylexLayersBefore keeps the reset and opaque skin layers below StyleX. */
export const stylexLayersBefore = [
  'reset',
  'theme',
  'base',
  'skin',
  'components',
  'utilities',
] as const

/** stylexPriority reads a stable compiler priority from a qualified layer name. */
export function stylexPriority(name: string): number | undefined {
  const match = /^stylex\.priority(\d+(?:_\d+)?)(?:__.*)?$/.exec(name)
  return match ? Number(match[1].replace('_', '.')) : undefined
}

// decodeLayerPart reverses the compiler's CSS identifier encoding.
function decodeLayerPart(part = ''): string {
  return part.replace(/_([0-9a-f]{4})/g, (_, code: string) =>
    String.fromCharCode(Number.parseInt(code, 16)),
  )
}

// compareLayers preserves compiler property and responsive ordering across module boundaries.
function compareLayers(left: string, right: string): number {
  // Compare numeric priorities before the property buckets within each priority.
  const priority = stylexPriority(left)! - stylexPriority(right)!
  if (priority) return priority
  const a = left.split('__').slice(1).map(decodeLayerPart)
  const b = right.split('__').slice(1).map(decodeLayerPart)
  const property = (a[0] ?? '').localeCompare(b[0] ?? '')
  if (property) return property

  // Like-unit min bounds ascend and max bounds descend; unparsed queries follow widths.
  if (a.length === 1 || b.length === 1) return b.length - a.length
  const context = (a[1] ?? '').localeCompare(b[1] ?? '')
  if (context) return context
  if (a[2] !== b[2]) return a[2] === 'min' ? -1 : 1
  return a[2] === 'min'
    ? Number(a[3]) - Number(b[3])
    : Number(b[3]) - Number(a[3])
}

/** stylexLayerOrder declares the complete compiler bucket order before stylesheet rules. */
export function stylexLayerOrder(layers: Iterable<string>): string {
  const names = [...new Set(layers)].sort(compareLayers)
  return names.length
    ? `@layer ${[...stylexLayersBefore, ...names].join(',')};`
    : ''
}
