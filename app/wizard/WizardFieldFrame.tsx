import { type ReactNode } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import { colors as themeColors } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'block',
    fontSize: '.65rem',
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

export interface WizardFieldFrameProps {
  label: ReactNode
  help?: ReactNode
  fieldClassName?: string
  labelClassName?: string
  children: ReactNode
}

export function WizardFieldFrame({
  label,
  help,
  fieldClassName,
  labelClassName,
  children,
}: WizardFieldFrameProps) {
  return (
    <label
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1.5 */ cn(
          'block min-w-0 space-y-1.5',
          fieldClassName,
        )
      }
    >
      <span
        className={
          /* TODO(stylex): dynamic class or forwarded className */ cn(
            'text-foreground-alt/70 block micro-label font-medium',
            labelClassName,
          )
        }
      >
        {label}
      </span>
      {children}
      {help && <span sx={stylexStyles.rule1}>{help}</span>}
    </label>
  )
}
