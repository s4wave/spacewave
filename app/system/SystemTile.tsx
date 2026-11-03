import * as ownerStylex from '@stylexjs/stylex'
import { cn as ownerClasses } from '@s4wave/web/style/utils.js'
import { groupMarker as ownergroupMarker } from '@s4wave/web/style/relations.stylex.js'
import * as stylex from '@stylexjs/stylex'
import { groupMarker } from '../../web/style/relations.stylex.js'
import { colors } from '../../web/style/tokens.stylex.js'
import type { ReactNode, Ref } from 'react'
import { LuMaximize2 } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import { toneDotClass, toneLabel } from './tone.js'
import type { SystemTone } from './useSystemModel.js'

import {
  layout as themeLayout,
  radii as themeRadii,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

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
  className,
  buttonRef,
  onOpen,
  children,
}: {
  title: string
  icon: ReactNode
  tone: SystemTone
  headline: ReactNode
  className?: string
  buttonRef?: Ref<HTMLButtonElement>
  onOpen: () => void
  children?: ReactNode
}) {
  return (
    <section
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: has-[button:focus-visible]:ring-2 */ cn(
          ownerClasses(
            'group border-foreground/8 bg-background-card/30 relative flex min-h-36 flex-col rounded-lg border p-4 transition-colors duration-150',
            ownerStylex.props(ownergroupMarker).className,
          ),
          'hover:border-foreground/15 hover:bg-background-card/60',
          'has-[button:focus-visible]:ring-brand/50 has-[button:focus-visible]:ring-2',
          className,
          stylex.props(groupMarker).className,
        )
      }
    >
      <h2 sx={stylexStyles.rule1}>
        <span
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: [&>svg]:size-3.5 */ 'shrink-0 [&>svg]:size-3.5'
          }
          aria-hidden="true"
        >
          {icon}
        </span>
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
          className={
            /* TODO(stylex): dynamic class or forwarded className */ cn(
              'size-1.5 rounded-full',
              toneDotClass[tone],
            )
          }
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
