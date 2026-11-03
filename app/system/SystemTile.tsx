import { tileIconMarker, tileSurfaceStyles } from './system-tile.stylex.js'
import { IconStylesProvider } from '@s4wave/web/images/icons.js'

import * as ownerStylex from '@stylexjs/stylex'

import * as stylex from '@stylexjs/stylex'
import { groupMarker } from '../../web/style/relations.stylex.js'
import { colors } from '../../web/style/tokens.stylex.js'
import type { ReactNode, Ref } from 'react'
import { LuMaximize2 } from '@s4wave/web/images/icons.js'

import { toneDotStyles, toneLabel } from './tone.js'
import type { SystemTone } from './useSystemModel.js'

import {
  layout as themeLayout,
  radii as themeRadii,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = ownerStylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    minHeight: `calc(${themeLayout['--spacing']} * 36)`,
    flexDirection: 'column',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${colors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${colors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${colors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${colors['--color-background-card']} 60%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

const stylexStyles2 = ownerStylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '3.40282e38px',
  },
})

const stylexStyles = ownerStylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    color: `color-mix(in oklab, ${colors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule2: {
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    textTransform: 'uppercase',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::after': {
      content: 'var(--tw-content)',
      position: 'absolute',
      inset: 0,
      borderRadius: themeRadii['--radius-lg'],
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    textAlign: { default: null, '@media (pointer: coarse)': 'left' },
  },
  rule3: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
  rule4: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: colors['--color-foreground'],
  },
  rule5: {
    marginTop: 'auto',
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${colors['--color-foreground-alt']} 70%, transparent)`,
  },
})

// SystemTile is one live subsystem tile of the system dashboard. Its title
// button stretches over the whole tile, so a click anywhere opens the
// subsystem inspector while the readouts stay ordinary markup.
export function SystemTile({
  title,
  icon,
  tone,
  headline,
  sx,
  buttonRef,
  onOpen,
  children,
}: {
  title: string
  icon: ReactNode
  tone: SystemTone
  headline: ReactNode
  sx?: stylex.StyleXStyles
  className?: never
  buttonRef?: Ref<HTMLButtonElement>
  onOpen: () => void
  children?: ReactNode
}) {
  return (
    <section
      sx={[
        [[stylexStyles3.rule1, groupMarker2]],
        [tileSurfaceStyles.focus, sx],
      ]}
    >
      <h2 sx={stylexStyles.rule1}>
        <IconStylesProvider requiredSx={tileSurfaceStyles.icon}>
          <span
            sx={[tileSurfaceStyles.iconWrap, tileIconMarker]}
            aria-hidden="true"
          >
            {icon}
          </span>
        </IconStylesProvider>
        <button
          ref={buttonRef}
          type="button"
          onClick={onOpen}
          sx={stylexStyles.rule2}
        >
          {title}
          <span sx={stylexStyles.rule3}>
            , {toneLabel[tone]}. Open inspector.
          </span>
        </button>
        <span
          sx={[[stylexStyles2.rule1], [toneDotStyles[tone]]]}
          aria-hidden="true"
        />
        <LuMaximize2 sx={tileIconStyles.icon} aria-hidden="true" />
      </h2>

      <div sx={stylexStyles.rule4}>{headline}</div>

      {children && <div sx={stylexStyles.rule5}>{children}</div>}
    </section>
  )
}

const tileIconStyles = stylex.create({
  icon: {
    color: `color-mix(in oklab, ${colors['--color-foreground-alt']} 50%, transparent)`,
    marginLeft: 'auto',
    width: 14,
    height: 14,
    transitionProperty: 'opacity',
    transitionTimingFunction:
      'var(--tw-ease,var(--default-transition-timing-function))',
    transitionDuration: 'var(--tw-duration,var(--default-transition-duration))',
    opacity: {
      default: 0,
      [stylex.when.ancestor(':has(button:focus-visible)', groupMarker)]: 1,
      '@media (hover:hover)': {
        default: null,
        [stylex.when.ancestor(':hover', groupMarker)]: 1,
      },
    },
  },
})
