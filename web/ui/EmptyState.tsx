import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { ReactNode } from 'react'
import { LuFolderOpen, LuPlus } from '@s4wave/web/images/icons.js'

import { Button } from './button.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    textAlign: 'center',
  },
  rule2: { padding: `calc(${themeLayout['--spacing']} * 4)` },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule4: { padding: `calc(${themeLayout['--spacing']} * 8)` },
  rule5: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-muted']} 50%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 14)`,
    width: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule8: { color: themeColors['--color-foreground-alt'] },
  rule9: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule10: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule13: {
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule15: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule16: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
  rule17: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginRight: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface EmptyStateProps {
  icon?: ReactNode
  title: string
  description?: string
  action?: {
    label: string
    onClick: () => void
  }
  sx?: ComponentStyles
  className?: never
  variant?: 'default' | 'compact' | 'panel'
}

// EmptyState renders a placeholder for empty lists or panels.
export function EmptyState({
  icon,
  title,
  description,
  action,
  sx,
  variant = 'default',
}: Omit<EmptyStateProps, 'className'> & { sx?: ComponentStyles }) {
  const isCompact = variant === 'compact'

  return (
    <div
      sx={[
        stylexStyles.rule1,
        isCompact
          ? stylexStyles.rule2
          : variant === 'panel'
            ? stylexStyles.rule3
            : stylexStyles.rule4,
        sx,
      ]}
    >
      <div
        sx={[
          stylexStyles.rule5,
          isCompact ? stylexStyles.rule6 : stylexStyles.rule7,
        ]}
      >
        {icon ?? (
          <LuFolderOpen
            sx={[
              stylexStyles.rule8,
              isCompact ? stylexStyles.rule9 : stylexStyles.rule10,
            ]}
          />
        )}
      </div>
      <h3
        sx={[
          stylexStyles.rule11,
          isCompact ? stylexStyles.rule12 : stylexStyles.rule13,
        ]}
      >
        {title}
      </h3>
      {description && (
        <p
          sx={[
            stylexStyles.rule8,
            isCompact ? stylexStyles.rule14 : stylexStyles.rule12,
          ]}
        >
          {description}
        </p>
      )}
      {action && (
        <Button
          variant="outline"
          size={isCompact ? 'sm' : 'default'}
          onClick={action.onClick}
          sx={isCompact ? stylexStyles.rule15 : stylexStyles.rule16}
        >
          <LuPlus sx={stylexStyles.rule17} />
          {action.label}
        </Button>
      )}
    </div>
  )
}
