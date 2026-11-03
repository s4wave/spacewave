import * as ownerStylex from '@stylexjs/stylex'

import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
import { useCallback } from 'react'

import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = ownerStylex.create({
  rule1: {
    padding: 0,
    backgroundColor: {
      default: '#0000',
      ':focus': themeColors['--color-navlink-selection'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-navlink-selection'],
      },
    },
    fontFamily: themeFonts['--font-heading'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    whiteSpace: 'nowrap',
    minHeight: { default: 24, '@media (pointer: coarse)': 44 },
  },
  rule2: {
    color: {
      default: themeColors['--color-navlink-text'],
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-navlink-text-hover'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = ownerStylex.create({
  rule1: {
    color: themeColors['--color-navlink-bracket'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]: 'none',
      },
      [ownerStylex.when.ancestor(':is(:focus)', groupMarker3)]: 'none',
    },
    paddingRight: themeLayout['--spacing-navlink-bracket'],
  },
  rule2: {
    color: themeColors['--color-navlink-bracket'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [ownerStylex.when.ancestor(':is(:hover)', groupMarker3)]: 'none',
      },
      [ownerStylex.when.ancestor(':is(:focus)', groupMarker3)]: 'none',
    },
    paddingLeft: themeLayout['--spacing-navlink-bracket'],
  },
})

export interface NavigationItem {
  text: string
  textSx?: ownerStylex.StyleXStyles
  sx?: ownerStylex.StyleXStyles
  className?: never
  onClick?: () => void
}

export function NavigationLink({ text, textSx, sx, onClick }: NavigationItem) {
  const handleNavigationSelect = useCallback(() => onClick?.(), [onClick])

  return (
    <button
      type="button"

      sx={[stylexStyles2.rule1, [ownergroupMarker, sx]]}
      onClick={handleNavigationSelect}
    >
      <span sx={stylexStyles.rule1}>[</span>
      <span sx={[stylexStyles2.rule2, textSx]}>{text}</span>
      <span sx={stylexStyles.rule2}>]</span>
    </button>
  )
}
