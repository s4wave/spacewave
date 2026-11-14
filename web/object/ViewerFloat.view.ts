// The floating controls over a viewer's content: when they show.

type ViewerFloat = {
  media: string
  active: boolean
  focus: string
}

export const title = 'Floating controls: shown or faded over the content'

// visible mirrors the spec's Visible.
const visible = (v: ViewerFloat) =>
  v.media !== 'moving' || v.active || v.focus === 'inside'

// state draws two regions: the content and the floats over it.
export const state = (v: ViewerFloat) => ({
  Content: v.media,
  Floats: visible(v) ? 'shown' : 'faded',
})

const steps: Record<string, string> = {
  Play: 'the video plays',
  Pause: 'the video pauses',
  Fail: 'the media fails',
  Touch: 'moves the pointer or presses a key',
  Idle: 'the pointer goes still or away',
  FocusIn: 'tabs into the floats',
  FocusOut: 'tabs out of the floats',
}

export const step = (action: string, from: ViewerFloat, to: ViewerFloat) => {
  const a = state(from)
  const b = state(to)
  if (a.Content === b.Content && a.Floats === b.Floats) return null
  return steps[action] ?? null
}
