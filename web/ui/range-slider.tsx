import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import * as SliderPrimitive from '@radix-ui/react-slider'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    alignItems: 'center',
    display: 'flex',
    position: 'relative',
    touchAction: 'none',
    userSelect: 'none',
    width: '100%',
  },
  rule2: {
    borderRadius: '3.40282e38px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-primary']} 20%, transparent)`,
    flexGrow: 1,
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
  },
  rule3: {
    backgroundColor: themeColors['--color-primary'],
    position: 'absolute',
    height: '100%',
  },
  rule4: {
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-primary']} 50%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    display: 'block',
    opacity: { default: null, ':disabled': 0.5 },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    pointerEvents: { default: null, ':disabled': 'none' },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

function Slider({
  sx,
  ...props
}: Omit<
  React.ComponentPropsWithRef<typeof SliderPrimitive.Root>,
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <SliderPrimitive.Root {...stylexProps([stylexStyles.rule1, sx], props)}>
      <SliderPrimitive.Track {...stylex.props(stylexStyles.rule2)}>
        <SliderPrimitive.Range {...stylex.props(stylexStyles.rule3)} />
      </SliderPrimitive.Track>
      <SliderPrimitive.Thumb {...stylex.props(stylexStyles.rule4)} />
    </SliderPrimitive.Root>
  )
}
Slider.displayName = SliderPrimitive.Root.displayName

export { Slider }
