import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as TabsPrimitive from '@radix-ui/react-tabs'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  rootBase: { display: 'flex', flexDirection: 'column' },
  listBase: {
    padding: themeLayout['--spacing'],
    borderRadius: themeRadii['--radius-lg'],
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'inline-flex',
    justifyContent: 'flex-start',
    width: '100%',
  },
  triggerBase: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-ring']} 50%, transparent)`,
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(3px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow': {
      default: null,
      ':is([data-state="active"])':
        '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    },
    borderRadius: themeRadii['--radius-md'],
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    boxShadow: {
      default: null,
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':is([data-state="active"])':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    color: {
      default: null,
      ':is([data-state="active"])': themeColors['--color-foreground'],
    },
    display: 'inline-flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: { default: null, ':disabled': 0.5 },
    outlineColor: {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    outlineStyle: {
      default: null,
      ':focus-visible': themeUtilityState['--tw-outline-style'],
    },
    outlineWidth: { default: null, ':focus-visible': '1px' },
    pointerEvents: { default: null, ':disabled': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'color, box-shadow',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    whiteSpace: 'nowrap',
    minHeight: { default: '24px', '@media (pointer: coarse)': '44px' },
    minWidth: { default: '24px', '@media (pointer: coarse)': '44px' },
  },
  contentBase: { '--tw-outline-style': 'none', flex: 1, outlineStyle: 'none' },
  rootDebug: { gap: `calc(${themeLayout['--spacing']} * 5)` },
  root: { gap: `calc(${themeLayout['--spacing']} * 2)` },
  listDebug: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    height: 'auto',
    minWidth: 'max-content',
  },
  list: {
    backgroundColor: themeColors['--color-muted'],
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  triggerDebug: {
    borderColor: {
      default: '#0000',
      ':is([data-state="active"])': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    },
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: null,
      ':is([data-state="active"])': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    },
  },
  trigger: {
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: {
      default: null,
      ':is([data-state="active"])': themeColors['--color-background'],
    },
  },
  contentForm: {
    '--tabs-content-space': `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  contentFormTight: {
    '--tabs-content-space': `calc(${themeLayout['--spacing']} * 3)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

/** tabsContentItemStyles retains form spacing on the direct children owned by a caller. */
export const tabsContentItemStyles = stylex.create({
  item: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--tabs-content-space, 0px) * calc(1 - var(--tw-space-y-reverse)))',
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--tabs-content-space, 0px) * var(--tw-space-y-reverse))',
    },
  },
})

/** Tabs owns the selected tab and its panels. */
function Tabs({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof TabsPrimitive.Root> & {
    variant?: 'default' | 'debug'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      {...stylexProps(
        [
          styles.rootBase,
          variant === 'debug' ? styles.rootDebug : styles.root,
          sx,
        ],
        props,
      )}
    />
  )
}

/** TabsList groups the available tabs. */
function TabsList({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof TabsPrimitive.List> & {
    variant?: 'default' | 'debug'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      {...stylexProps(
        [
          styles.listBase,
          variant === 'debug' ? styles.listDebug : styles.list,
          sx,
        ],
        props,
      )}
    />
  )
}

/** TabsTrigger selects its associated panel. */
function TabsTrigger({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof TabsPrimitive.Trigger> & {
    variant?: 'default' | 'debug'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      {...stylexProps(
        [
          styles.triggerBase,
          variant === 'debug' ? styles.triggerDebug : styles.trigger,
          sx,
        ],
        props,
      )}
    />
  )
}

/** TabsContent renders one panel; form variants space direct children carrying tabsContentItemStyles.item. */
function TabsContent({
  sx,
  variant,
  ...props
}: Omit<
  React.ComponentProps<typeof TabsPrimitive.Content> & {
    variant?: 'default' | 'form' | 'formTight'
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <TabsPrimitive.Content
      data-slot="tabs-content"
      {...stylexProps(
        [
          styles.contentBase,
          variant === 'form'
            ? styles.contentForm
            : variant === 'formTight'
              ? styles.contentFormTight
              : undefined,
          sx,
        ],
        props,
      )}
    />
  )
}

export { Tabs, TabsContent, TabsList, TabsTrigger }
