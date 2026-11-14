// The side column and the Details popover: where a viewer's facts show.

type ViewerSide = {
  width: string
  side: string
  pop: string
}

export const title = 'Side column and Details: the facts by pane width'

// column names what the pane's left edge draws.
const column = (v: ViewerSide) => {
  if (v.width !== 'wide') return 'none'
  return v.side === 'open' ? 'facts, Collapse' : 'rail, Expand'
}

// state draws two regions: the side column and the Details press or popover.
export const state = (v: ViewerSide) => ({
  Column: column(v),
  Details:
    v.pop === 'open'
      ? 'popover with facts'
      : v.width === 'wide'
        ? 'none'
        : 'Details press',
})

const steps: Record<string, string> = {
  Resize: 'the pane is resized',
  Collapse: 'presses Collapse',
  Expand: 'presses Expand',
  OpenDetails: 'presses Details',
  Dismiss: 'presses Escape',
}

export const step = (action: string, from: ViewerSide, to: ViewerSide) => {
  const a = state(from)
  const b = state(to)
  if (a.Column === b.Column && a.Details === b.Details) return null
  return steps[action] ?? null
}
