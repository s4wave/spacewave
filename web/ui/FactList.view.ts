// A fact row: whether its value shows whole, and its copy press.

type FactList = {
  width: string
  len: string
  head: string
  copy: string
  clip: string
}

export const title = 'Fact row: the value by width, and copy'

// value names how the value cell draws at the list's width.
const value = (v: FactList) => {
  const fits = v.len === 'short' || v.width === 'roomy'
  const shape = fits
    ? 'whole'
    : v.head === 'long'
      ? 'head…tail'
      : 'head…tail end'
  return v.width === 'stacked' ? `label over value, ${shape}` : shape
}

const press: Record<string, string> = {
  idle: 'Copy',
  copied: 'Copied',
  failed: 'Copy failed',
}

// state draws two regions: the value cell and the copy press.
export const state = (v: FactList) => ({
  Value: value(v),
  Copy: press[v.copy] ?? v.copy,
})

const steps: Record<string, string> = {
  Resize: 'the pane is resized',
  Changed: 'another writer changes the value',
  Copied: 'presses Copy',
  Failed: 'presses Copy; the clipboard refuses',
  Reset: 'two seconds pass',
}

export const step = (action: string, from: FactList, to: FactList) => {
  const a = state(from)
  const b = state(to)
  if (a.Value === b.Value && a.Copy === b.Copy) return null
  return steps[action] ?? null
}
