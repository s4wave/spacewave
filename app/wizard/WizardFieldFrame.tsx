import type { ReactNode } from 'react'
import * as stylex from '@stylexjs/stylex'

import {
  fonts as themeFonts,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: { display: 'block', minWidth: 0 },
  rule2: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    display: 'block',
    fontSize: '.65rem',
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: 1.5,
  },
  rule3: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'block',
    fontSize: '.65rem',
    lineHeight: 'var(--leading-relaxed)',
  },
})

export interface WizardFieldFrameProps {
  label: ReactNode
  help?: ReactNode
  sx?: stylex.StyleXStyles
  labelSx?: stylex.StyleXStyles
  className?: never
  children: (controlSx: stylex.StyleXStyles) => ReactNode
}

// WizardFieldFrame keeps each control's label and help in the same label element.
export function WizardFieldFrame({
  label,
  help,
  sx,
  labelSx,
  children,
}: WizardFieldFrameProps) {
  return (
    <label sx={[stylexStyles.rule1, sx]}>
      <span sx={[stylexStyles.rule2, [fieldChildren.child, labelSx]]}>
        {label}
      </span>
      {children(fieldChildren.child)}
      {help && (
        <span sx={[stylexStyles.rule3, fieldChildren.child]}>{help}</span>
      )}
    </label>
  )
}

// fieldChildren preserves the original margins at the label, control and help owners.
const fieldChildren = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))',
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))',
    },
  },
})
