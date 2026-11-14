// The viewer bar's presses as the pane narrows and widens, and its More menu.

type Bar = {
  width: string
  fold: Record<string, string>
  present: string[]
  menu: string
  listed: string[]
}

export const title = 'Object viewer: the one-line bar'

const order = ['cramped', 'narrow', 'wide', 'full']

// wide is the 28rem band and up; full, from 42rem, removes the narrow press.
const wide = (v: Bar) => v.width === 'wide' || v.width === 'full'
const gone = (v: Bar, a: string) => v.width === 'full' && a === 'a'

const folded = (v: Bar) =>
  v.present.filter(
    (a) =>
      !gone(v, a) &&
      (v.fold[a] === 'menu' ||
        (v.fold[a] === 'md' && !wide(v)) ||
        (v.fold[a] === 'xs' && v.width === 'cramped')),
  ).length

export const state = (v: Bar) => {
  if (v.menu === 'open') return 'More open'
  const n = v.present.filter((a) => !gone(v, a)).length
  const f = folded(v)
  if (n === 0) return 'Only the title'
  if (f === 0) return 'Every press in the bar'
  if (f === n) return 'Title and More'
  return 'Some presses, then More'
}

export const step = (action: string, from: Bar, to: Bar) => {
  // Pressing or focusing a press changes nothing the bar draws.
  if (action === 'PressInline' || action === 'Focus') return null
  if (state(from) === state(to)) return null
  if (action === 'Resize') {
    return order.indexOf(to.width) > order.indexOf(from.width)
      ? 'pane widens'
      : 'pane narrows'
  }
  if (action === 'Change') return 'presses change'
  if (action === 'Open') return 'presses More'
  if (action === 'PickListed') return 'picks a press'
  if (action === 'Dismiss') return 'Escape or click away'
  return null
}
