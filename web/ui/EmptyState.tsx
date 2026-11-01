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
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    textAlign: 'center',
  },
  rule2: { padding: `calc(${themeLayout['--spacing']} * 4)` },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule4: { padding: `calc(${themeLayout['--spacing']} * 8)` },
  rule5: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-muted']} 50%, transparent)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 14)`,
    height: `calc(${themeLayout['--spacing']} * 14)`,
  },
  rule8: { color: themeColors['--color-foreground-alt'] },
  rule9: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule10: {
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule12: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: 'var(--text-base)',
    lineHeight: 'var(--tw-leading, var(--text-base--line-height))',
  },
  rule14: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule16: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
  rule17: {
    marginRight: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
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
            {...stylex.props([
              stylexStyles.rule8,
              isCompact ? stylexStyles.rule9 : stylexStyles.rule10,
            ])}
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
          <LuPlus {...stylex.props(stylexStyles.rule17)} />
          {action.label}
        </Button>
      )}
    </div>
  )
}
