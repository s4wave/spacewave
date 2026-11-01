import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  card: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-card'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-card-foreground'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  header: {
    container: 'card-header / inline-size',
    display: 'grid',
    gridAutoRows: 'min-content',
    gridTemplateRows: 'auto auto',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
  },
  headerBorder: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    paddingBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  title: {
    '--tw-leading': '1',
    lineHeight: 1,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
  },
  description: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-muted-foreground'],
  },
  action: {
    gridColumnStart: 2,
    gridRow: 'span 2 / span 2',
    gridRowStart: 1,
    alignSelf: 'flex-start',
    justifySelf: 'flex-end',
  },
  content: { paddingInline: `calc(${themeLayout['--spacing']} * 6)` },
  footer: {
    display: 'flex',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
  },
  footerBorder: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  headerAction: {
    gridTemplateColumns: {
      default: null,
      [stylex.when.descendant('[data-slot="card-action"]')]: '1fr auto',
    },
  },
})

function Card({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return <div data-slot="card" sx={[styles.card, sx]} {...props} />
}

// CardHeader keeps the action column and optional separator with its content.
function CardHeader({
  sx,
  bordered = false,
  ...props
}: Omit<React.ComponentProps<'div'> & { bordered?: boolean }, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="card-header"
      sx={[
        styles.header,
        styles.headerAction,
        !!bordered && styles.headerBorder,
        sx,
      ]}
      {...props}
    />
  )
}

function CardTitle({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return <div data-slot="card-title" sx={[styles.title, sx]} {...props} />
}

function CardDescription({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="card-description"
      sx={[styles.description, sx]}
      {...props}
    />
  )
}

function CardAction({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="card-action"
      {...stylex.props(styles.action, sx, stylex.defaultMarker())}
      {...props}
    />
  )
}

function CardContent({
  sx,
  ...props
}: Omit<React.ComponentProps<'div'>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return <div data-slot="card-content" sx={[styles.content, sx]} {...props} />
}

// CardFooter adds the top separator and its spacing when bordered is set.
function CardFooter({
  sx,
  bordered = false,
  ...props
}: Omit<React.ComponentProps<'div'> & { bordered?: boolean }, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div
      data-slot="card-footer"
      sx={[styles.footer, !!bordered && styles.footerBorder, sx]}
      {...props}
    />
  )
}

export {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
}
