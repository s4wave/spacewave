import type { SystemTone } from './useSystemModel.js'

import * as stylex from '@stylexjs/stylex'
import { colors as themeColors } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { backgroundColor: themeColors['--color-success'] },
  rule2: { backgroundColor: themeColors['--color-brand'] },
  rule3: { backgroundColor: themeColors['--color-warning'] },
  rule4: { backgroundColor: themeColors['--color-destructive'] },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule6: { color: themeColors['--color-success'] },
  rule7: { color: themeColors['--color-brand'] },
  rule8: { color: themeColors['--color-warning'] },
  rule9: { color: themeColors['--color-destructive'] },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

// toneDotStyles is the indicator fill for each system tone.
export const toneDotStyles: Record<SystemTone, stylex.StyleXStyles> = {
  nominal: stylexStyles.rule1,
  active: stylexStyles.rule2,
  warning: stylexStyles.rule3,
  error: stylexStyles.rule4,
  pending: stylexStyles.rule5,
}

// toneTextStyles is the text color that carries each system tone.
export const toneTextStyles: Record<SystemTone, stylex.StyleXStyles> = {
  nominal: stylexStyles.rule6,
  active: stylexStyles.rule7,
  warning: stylexStyles.rule8,
  error: stylexStyles.rule9,
  pending: stylexStyles.rule10,
}

// toneLabel names each system tone for assistive technology.
export const toneLabel: Record<SystemTone, string> = {
  nominal: 'Nominal',
  active: 'Working',
  warning: 'Needs attention',
  error: 'Fault',
  pending: 'Reading',
}
