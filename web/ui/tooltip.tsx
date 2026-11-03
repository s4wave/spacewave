import { keyframeNames } from '../style/tokens.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-enter-opacity': '0',
    '--tw-enter-scale': '.95',
    '--tw-enter-translate-x': {
      default: null,
      ':is([data-side="left"])': '.5rem',
      ':is([data-side="right"])': '-.5rem',
    },
    '--tw-enter-translate-y': {
      default: null,
      ':is([data-side="bottom"])': '-.5rem',
      ':is([data-side="top"])': '.5rem',
    },
    '--tw-exit-opacity': { default: null, ':is([data-state="closed"])': '0' },
    '--tw-exit-scale': { default: null, ':is([data-state="closed"])': '.95' },
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    animationDelay: { default: '0s', ':is([data-state="closed"])': '0s' },
    animationDirection: {
      default: 'normal',
      ':is([data-state="closed"])': 'normal',
    },
    animationDuration: {
      default: '.15s',
      ':is([data-state="closed"])': '.15s',
    },
    animationFillMode: {
      default: 'both',
      ':is([data-state="closed"])': 'both',
    },
    animationIterationCount: { default: 1, ':is([data-state="closed"])': 1 },
    animationName: {
      default: keyframeNames['shadcn-enter'],
      ':is([data-state="closed"])': keyframeNames['shadcn-exit'],
    },
    animationPlayState: {
      default: 'running',
      ':is([data-state="closed"])': 'running',
    },
    animationTimeline: {
      default: 'auto',
      ':is([data-state="closed"])': 'auto',
    },
    animationTimingFunction: {
      default: 'ease-out',
      ':is([data-state="closed"])': 'ease-in',
    },
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: 'color-mix(in oklab, var(--color-black) 90%, transparent)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-white'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textWrap: 'balance',
    transformOrigin: 'var(--radix-tooltip-content-transform-origin)',
    zIndex: 50,
    width: 'fit-content',
  },
  rule2: {
    '--tw-translate-y': 'calc(-50% - 2px)',
    fill: 'color-mix(in oklab, var(--color-black) 90%, transparent)',
    borderRadius: '2px',
    rotate: '45deg',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    zIndex: 50,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
})

function TooltipProvider({
  delayDuration = 0,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delayDuration={delayDuration}
      {...props}
    />
  )
}

function Tooltip({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return (
    <TooltipProvider>
      <TooltipPrimitive.Root data-slot="tooltip" {...props} />
    </TooltipProvider>
  )
}

function TooltipTrigger({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />
}

function TooltipContent({
  sx,
  sideOffset = 0,
  children,
  ...props
}: Omit<React.ComponentProps<typeof TooltipPrimitive.Content>, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        {...stylexProps([stylexStyles.rule1, sx], props)}
      >
        {children}
        <TooltipPrimitive.Arrow {...stylex.props(stylexStyles.rule2)} />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  )
}

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider }
