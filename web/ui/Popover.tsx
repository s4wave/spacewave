import type { StyleXStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as PopoverPrimitive from '@radix-ui/react-popover'

import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const styles = stylex.create({
  typography: {
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
  content: {
    zIndex: 50,
    width: `calc(${themeLayout['--spacing']} * 72)`,
    transformOrigin: 'var(--radix-popover-content-transform-origin)',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-popover-border'],
    backgroundColor: themeColors['--color-popover'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-popover-text'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    outlineOffset: { default: null, '@media (forced-colors: active)': '2px' },
    outline: {
      default: null,
      '@media (forced-colors: active)': '2px solid #0000',
    },
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-side="bottom"])': '-.5rem',
      ':is([data-side="top"])': '.5rem',
    },
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-side="left"])': '.5rem',
      ':is([data-side="right"])': '-.5rem',
    },
    animationName: {
      default: null,
      ':is([data-state="closed"])': 'shadcn-exit',
      ':is([data-state="open"])': 'shadcn-enter',
    },
    animationDuration: {
      default: null,
      ':is([data-state="closed"])': '.15s',
      ':is([data-state="open"])': '.15s',
    },
    animationTimingFunction: {
      default: null,
      ':is([data-state="closed"])': 'ease-in',
      ':is([data-state="open"])': 'ease-out',
    },
    animationIterationCount: {
      default: null,
      ':is([data-state="closed"])': 1,
      ':is([data-state="open"])': 1,
    },
    animationDirection: {
      default: null,
      ':is([data-state="closed"])': 'normal',
      ':is([data-state="open"])': 'normal',
    },
    animationPlayState: {
      default: null,
      ':is([data-state="closed"])': 'running',
      ':is([data-state="open"])': 'running',
    },
    animationDelay: {
      default: null,
      ':is([data-state="closed"])': '0s',
      ':is([data-state="open"])': '0s',
    },
    animationFillMode: {
      default: null,
      ':is([data-state="closed"])': 'both',
      ':is([data-state="open"])': 'both',
    },
    animationTimeline: {
      default: null,
      ':is([data-state="closed"])': 'auto',
      ':is([data-state="open"])': 'auto',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-enter-opacity': { default: null, ':is([data-state="open"])': '0' },
    '--tw-enter-scale': { default: null, ':is([data-state="open"])': '.95' },
  },
  status: {
    zIndex: 50,
    width: `calc(${themeLayout['--spacing']} * 80)`,
    maxWidth: themeLayout['--max-width-viewport-control'],
    borderRadius: themeRadii['--radius-lg'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    backgroundColor: themeColors['--color-background-card'],
    padding: 0,
    color: themeColors['--color-foreground'],
    '--tw-shadow':
      '0 20px 25px -5px var(--tw-shadow-color, #0000001a), 0 8px 10px -6px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-md))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  compact: {
    width: `calc(${themeLayout['--spacing']} * 72)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

function Popover({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />
}

function PopoverTrigger({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
}

function PopoverContent({
  sx,
  variant,
  align = 'center',
  sideOffset = 4,
  ...props
}: Omit<
  React.ComponentProps<typeof PopoverPrimitive.Content> & {
    variant?: 'default' | 'status' | 'compact'
  },
  'className'
> & { sx?: StyleXStyles; className?: never }) {
  // Keep portal ownership and typography with the current application and variant.
  const environment = useAppEnvironment()
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-spacewave-app={environment.id || undefined}
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        {...stylex.props([
          styles.content,
          variant !== 'status' && styles.typography,
          variant === 'status' && styles.status,
          variant === 'compact' && styles.compact,
          sx,
        ])}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

function PopoverAnchor({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Anchor>) {
  return <PopoverPrimitive.Anchor data-slot="popover-anchor" {...props} />
}

export { Popover, PopoverTrigger, PopoverContent, PopoverAnchor }
