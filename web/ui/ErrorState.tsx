import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { type ReactNode } from 'react'
import { LuCircleAlert, LuRefreshCw } from '@s4wave/web/images/icons.js'

import { Button } from './button.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
  },
  rule2: {
    backgroundColor: themeColors['--color-error-bg'],
    height: `calc(${themeLayout['--spacing']} * 16)`,
    width: `calc(${themeLayout['--spacing']} * 16)`,
  },
  rule3: {
    backgroundColor: themeColors['--color-error-bg'],
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: { color: themeColors['--color-error'] },
  rule5: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule7: { textAlign: 'center' },
  rule8: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
  rule9: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule10: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-error-text'],
    fontWeight: 'var(--font-weight-semibold)',
  },
  rule11: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule13: {
    color: themeColors['--color-foreground-alt'],
    marginTop: themeLayout['--spacing'],
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    borderColor: themeColors['--color-error-border'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-error-bg'],
      },
    },
    color: themeColors['--color-error'],
  },
  rule16: { marginTop: `calc(${themeLayout['--spacing']} * 6)` },
  rule17: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule18: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: themeColors['--color-error-border'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-error-bg'],
    display: 'flex',
  },
  rule19: {
    color: themeColors['--color-error'],
    flexShrink: '0',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule20: {
    color: themeColors['--color-error-text'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule21: {
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-error-bg'],
      },
    },
    color: themeColors['--color-error'],
    flexShrink: '0',
    marginLeft: 'auto',
  },
  rule22: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule23: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    flex: '1',
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    height: '100%',
    minHeight: '0',
    width: '100%',
  },
  rule24: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
  },
})

interface ErrorStateProps {
  title?: string
  message: string
  onRetry?: () => void
  sx?: ComponentStyles
  className?: never
  variant?: 'inline' | 'card' | 'fullscreen'
  children?: ReactNode
}

// ErrorState renders an error message with optional retry functionality.
export function ErrorState({
  title = 'Error',
  message,
  onRetry,
  sx,
  variant = 'card',
  children,
}: Omit<ErrorStateProps, 'className'> & { sx?: ComponentStyles }) {
  const content = (
    <>
      <div
        sx={[
          stylexStyles.rule1,
          variant === 'fullscreen' ? stylexStyles.rule2 : stylexStyles.rule3,
        ]}
      >
        <LuCircleAlert
          sx={[
            stylexStyles.rule4,
            variant === 'fullscreen' ? stylexStyles.rule5 : stylexStyles.rule6,
          ]}
        />
      </div>
      <div
        sx={[
          stylexStyles.rule7,
          variant === 'fullscreen' ? stylexStyles.rule8 : stylexStyles.rule9,
        ]}
      >
        <h3
          sx={[
            stylexStyles.rule10,
            variant === 'fullscreen'
              ? stylexStyles.rule11
              : stylexStyles.rule12,
          ]}
        >
          {title}
        </h3>
        <p
          sx={[
            stylexStyles.rule13,
            variant === 'fullscreen'
              ? stylexStyles.rule12
              : stylexStyles.rule14,
          ]}
        >
          {message}
        </p>
      </div>
      {onRetry && (
        <Button
          variant="outline"
          size={variant === 'fullscreen' ? 'default' : 'sm'}
          onClick={onRetry}
          sx={[
            stylexStyles.rule15,
            variant === 'fullscreen' ? stylexStyles.rule16 : stylexStyles.rule8,
          ]}
        >
          <LuRefreshCw sx={stylexStyles.rule17} />
          Retry
        </Button>
      )}
      {children}
    </>
  )

  if (variant === 'inline') {
    return (
      <div sx={[stylexStyles.rule18, sx]}>
        <LuCircleAlert sx={stylexStyles.rule19} />
        <p sx={stylexStyles.rule20}>{message}</p>
        {onRetry && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onRetry}
            sx={stylexStyles.rule21}
          >
            <LuRefreshCw sx={stylexStyles.rule22} />
          </Button>
        )}
      </div>
    )
  }

  if (variant === 'fullscreen') {
    return <div sx={[stylexStyles.rule23, sx]}>{content}</div>
  }

  return <div sx={[stylexStyles.rule24, sx]}>{content}</div>
}
