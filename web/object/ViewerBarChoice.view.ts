// The viewer bar's choice as the pane narrows and widens, and its menu.

type Choice = {
  width: string
  menu: string
}

export const title = 'Object viewer: the bar choice'

export const state = (v: Choice) => {
  if (v.menu === 'open') return 'Menu open'
  return v.width === 'wide' ? 'Options in the bar' : 'One press'
}

export const step = (action: string, from: Choice, to: Choice) => {
  if (action === 'Focus') return null
  if (state(from) === state(to)) return null
  if (action === 'Resize') {
    return to.width === 'wide' ? 'pane widens' : 'pane narrows'
  }
  if (action === 'Open') return 'presses the choice'
  if (action === 'Pick') return 'picks an option'
  if (action === 'Dismiss') return 'Escape or click away'
  return null
}
