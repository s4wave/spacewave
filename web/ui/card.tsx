import { stylexProps } from '@s4wave/web/style/props.js'
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
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    backgroundColor: themeColors['--color-card'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-card-foreground'],
    display: 'flex',
    flexDirection: 'column',
  },
  header: {
    container: 'card-header / inline-size',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'flex-start',
    display: 'grid',
    gridAutoRows: 'min-content',
    gridTemplateRows: 'auto auto',
  },
  headerBorder: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    paddingBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  title: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': '1',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 1,
  },
  description: {
    color: themeColors['--color-muted-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  action: {
    alignSelf: 'flex-start',
    gridColumnStart: 2,
    gridRowEnd: 'span 2',
    gridRowStart: 1,
    justifySelf: 'flex-end',
  },
  content: { paddingInline: `calc(${themeLayout['--spacing']} * 6)` },
  footer: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
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
  return <div data-slot="card" {...stylexProps([styles.card, sx], props)} />
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
      {...stylexProps(
        [
          styles.header,
          styles.headerAction,
          !!bordered && styles.headerBorder,
          sx,
        ],
        props,
      )}
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
  return (
    <div data-slot="card-title" {...stylexProps([styles.title, sx], props)} />
  )
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
      {...stylexProps([styles.description, sx], props)}
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
      {...stylexProps([styles.action, sx, stylex.defaultMarker()], props)}
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
  return (
    <div
      data-slot="card-content"
      {...stylexProps([styles.content, sx], props)}
    />
  )
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
      {...stylexProps(
        [styles.footer, !!bordered && styles.footerBorder, sx],
        props,
      )}
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
