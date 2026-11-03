import * as stylex from '@stylexjs/stylex'
import { colors, utilityState } from './tokens.stylex.js'

/** rowDividers styles the nonfinal physical rows owned by a list or transform card. */
export const rowDividers = stylex.create({
  regular: {
    '--tw-divide-y-reverse': 0,
    borderColor: `color-mix(in oklab, ${colors['--color-foreground']} 6%, transparent)`,
    borderBottomStyle: utilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderTopStyle: utilityState['--tw-border-style'],
    borderTopWidth: 0,
  },
})
