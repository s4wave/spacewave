import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { type ReactNode } from 'react'
import { LuCircleAlert, LuRefreshCw } from 'react-icons/lu'

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
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 16)`,
    height: `calc(${themeLayout['--spacing']} * 16)`,
    backgroundColor: themeColors['--color-error-bg'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    backgroundColor: themeColors['--color-error-bg'],
  },
  rule4: { color: themeColors['--color-error'] },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule7: { textAlign: 'center' },
  rule8: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
  rule9: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  rule10: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-error-text'],
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
    marginTop: themeLayout['--spacing'],
    color: themeColors['--color-foreground-alt'],
  },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    borderColor: themeColors['--color-error-border'],
    color: themeColors['--color-error'],
    '@media (hover: hover)': {
      ':hover': { backgroundColor: themeColors['--color-error-bg'] },
    },
  },
  rule16: { marginTop: `calc(${themeLayout['--spacing']} * 6)` },
  rule17: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule18: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: 'solid',
    borderWidth: '1px',
    borderColor: themeColors['--color-error-border'],
    backgroundColor: themeColors['--color-error-bg'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule19: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: '0',
    color: themeColors['--color-error'],
  },
  rule20: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-error-text'],
  },
  rule21: {
    marginLeft: 'auto',
    flexShrink: '0',
    color: themeColors['--color-error'],
    '@media (hover: hover)': {
      ':hover': { backgroundColor: themeColors['--color-error-bg'] },
    },
  },
  rule22: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule23: {
    display: 'flex',
    height: '100%',
    minHeight: '0',
    width: '100%',
    flex: '1',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule24: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
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
          {...stylex.props([
            stylexStyles.rule4,
            variant === 'fullscreen' ? stylexStyles.rule5 : stylexStyles.rule6,
          ])}
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
          {...stylex.props([
            stylexStyles.rule15,
            variant === 'fullscreen' ? stylexStyles.rule16 : stylexStyles.rule8,
          ])}
        >
          <LuRefreshCw {...stylex.props(stylexStyles.rule17)} />
          Retry
        </Button>
      )}
      {children}
    </>
  )

  if (variant === 'inline') {
    return (
      <div sx={[stylexStyles.rule18, sx]}>
        <LuCircleAlert {...stylex.props(stylexStyles.rule19)} />
        <p sx={stylexStyles.rule20}>{message}</p>
        {onRetry && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onRetry}
            {...stylex.props(stylexStyles.rule21)}
          >
            <LuRefreshCw {...stylex.props(stylexStyles.rule22)} />
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
