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
    '--tw-duration': '.15s',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: {
      default: `color-mix(in oklab, ${colors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${colors['--color-foreground']} 15%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${colors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${colors['--color-background-card']} 60%, transparent)`,
      },
    },
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    minHeight: `calc(${themeLayout['--spacing']} * 36)`,
  },
})

const stylexStyles2 = ownerStylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    width: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles = ownerStylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${colors['--color-foreground-alt']} 60%, transparent)`,
    display: 'flex',
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-outline-style': 'none',
    '--tw-tracking': 'var(--tracking-wider)',
    fontSize: themeLayout['--text-metadata'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    outlineStyle: 'none',
    textAlign: { default: null, '@media (pointer: coarse)': 'left' },
    textTransform: 'uppercase',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    '::after': {
      inset: 0,
      borderRadius: themeRadii['--radius-lg'],
      content: 'var(--tw-content)',
      position: 'absolute',
    },
  },
  rule3: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: colors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    color: `color-mix(in oklab, ${colors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: 'auto',
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
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
    opacity: {
      default: 0,
      [stylex.when.ancestor(':has(button:focus-visible)', groupMarker)]: 1,
      '@media (hover:hover)': {
        default: null,
        [stylex.when.ancestor(':hover', groupMarker)]: 1,
      },
    },
    transitionDuration: 'var(--tw-duration,var(--default-transition-duration))',
    transitionProperty: 'opacity',
    transitionTimingFunction:
      'var(--tw-ease,var(--default-transition-timing-function))',
    height: 14,
    marginLeft: 'auto',
    width: 14,
  },
})
