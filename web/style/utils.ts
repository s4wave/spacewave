import { clsx, type ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Custom font-size utilities defined as CSS variables (--text-*) in app.css.
// Without this config, twMerge treats text-metadata (font-size) and
// text-brand (color) as the same group and drops one.
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [
        'text-metadata',
        'text-devtools',
        'text-ui',
        'text-file-browser',
        'text-dashboard',
        'text-topbar-menu',
        'text-header',
      ],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
