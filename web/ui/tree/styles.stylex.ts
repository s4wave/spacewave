import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../style/tokens.stylex.js'

/** treeRowActionIconStyles sets the dimensions at each action SVG's owner. */
export const treeRowActionIconStyles = stylex.create({
  icon: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})
