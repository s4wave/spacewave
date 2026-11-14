import { createCn } from 'cnfast'

// textSizes are the font-size tokens app.css adds to the theme as --text-*.
// cn must know them as sizes; otherwise it reads text-header as a color and
// drops it whenever a text color follows in the same call.
export const textSizes = [
  'dashboard',
  'devtools',
  'file-browser',
  'header',
  'landing-prompt',
  'metadata',
  'topbar-menu',
  'ui',
  'viewer-title',
]

/** cn joins class names and lets a later Tailwind class replace a conflicting one. */
export const cn = createCn({ extend: { theme: { text: textSizes } } })
