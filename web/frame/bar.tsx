import { cn } from '@s4wave/web/style/utils.js'
import { ControlStylesProvider } from '@s4wave/web/style/ControlStylesProvider.js'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'
import { iconRequired } from '@s4wave/web/style/icon-defaults.stylex.js'
import { viewerBarStyles } from '@s4wave/web/style/viewer-bar.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import React, { CSSProperties } from 'react'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    height: themeLayout['--spacing-bar'],
    minHeight: themeLayout['--spacing-bar'],
    width: '100%',
    flexShrink: 0,
    flexDirection: 'row',
    flexWrap: 'nowrap',
    gap: 0,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-bar'],
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-bar-font'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    textDecorationLine: 'none',
    outlineStyle: themeUtilityState['--tw-outline-style'],
    outlineWidth: 0,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.12s',
    '--tw-duration': '.12s',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    minWidth: 0,
    flexGrow: 1,
    overflow: 'hidden',
    textAlign: 'left',
  },
  rule2: { display: 'flex', flexShrink: 0, overflow: 'hidden' },
  rule3: {
    '::after': {
      content: '""',
      pointerEvents: 'none',
      position: 'absolute',
      top: 0,
      left: 0,
      height: `calc(${themeLayout['--spacing']} * .25)`,
      width: '100%',
      backgroundColor: themeColors['--color-bar-border-top'],
    },
  },
})

// IBarProps are properties for a horizontal frame bar.
export interface IBarProps {
  // dark marks descendants that still use the shared dark theme variant.
  dark?: boolean
  className?: never
  // hidden hides the bar.
  hidden?: boolean
  // left are elements for the bottom bar left side.
  left?: React.ReactNode
  // right are elements for the bottom bar right side.
  right?: React.ReactNode
  // style are additional styles for the bar.
  style?: CSSProperties
  // sx adds styles to the root element.
  sx?: ComponentStyles
  // hideTopBorder hides the top border.
  hideTopBorder?: boolean
}

// Bar renders horizontal slots with the frame's native control requirements.
export function Bar({
  hidden,
  dark,
  sx,
  style,
  left,
  right,
  hideTopBorder,
}: IBarProps = {}) {
  if (hidden) return null
  const attributes = stylexProps([stylexStyles2.rule1, sx], { style })
  attributes.className = cn(attributes.className, dark && 'dark')
  return (
    <ControlStylesProvider styles={viewerBarStyles.required}>
      <IconStylesProvider requiredSx={iconRequired.viewerBar}>
        <div {...attributes}>
          <div sx={stylexStyles.rule1}>{left}</div>
          <div sx={[stylexStyles.rule2, viewerBarStyles.right]}>{right}</div>
          {!hideTopBorder ? <span sx={stylexStyles.rule3} /> : null}
        </div>
      </IconStylesProvider>
    </ControlStylesProvider>
  )
}
