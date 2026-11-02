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
    position: 'relative',
    display: 'flex',
    width: '100%',
    touchAction: 'none',
    alignItems: 'center',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: '100%',
    flexGrow: 1,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-primary']} 20%, transparent)`,
  },
  rule3: {
    position: 'absolute',
    height: '100%',
    backgroundColor: themeColors['--color-primary'],
  },
  rule4: {
    display: 'block',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-primary']} 50%, transparent)`,
    backgroundColor: themeColors['--color-background'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow: {
      default:
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
      ':focus-visible':
        'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-ring-shadow': {
      default: null,
      ':focus-visible':
        'var(--tw-ring-inset, ) 0 0 0 calc(1px + var(--tw-ring-offset-width)) var(--tw-ring-color, currentcolor)',
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': themeColors['--color-ring'],
    },
    '--tw-outline-style': { default: null, ':focus-visible': 'none' },
    outlineStyle: { default: null, ':focus-visible': 'none' },
    pointerEvents: { default: null, ':disabled': 'none' },
    opacity: { default: null, ':disabled': 0.5 },
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
