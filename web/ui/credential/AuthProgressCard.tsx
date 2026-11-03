import type { StyleXStyles } from '@stylexjs/stylex'
import { LuCheck, LuKeyRound, LuShield } from '@s4wave/web/images/icons.js'

import { ProgressBar } from '@s4wave/web/ui/loading/ProgressBar.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  radii as themeRadii,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../../style/tokens.stylex.js'

const styles = stylex.create({
  root: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  steps: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  header: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  iconBox: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  body: { flex: 1, minWidth: 0 },
  title: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  detail: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  progress: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  step: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(${themeLayout['--spacing']} * 1.5)`,
    },
    marginBlockStart: { default: null, ':not(:last-child)': 0 },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  activeLabel: { color: themeColors['--color-foreground'] },
  pendingLabel: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  stepIconBox: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  activeIcon: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  pendingIcon: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  stepIcon: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface AuthProgressCardProps {
  className?: never
  title: string
  detail: string
  steps: string[]
  sx?: StyleXStyles
}

// AuthProgressCard shows honest indeterminate progress for auth flows whose
// crypto and provider work does not expose reliable percent completion.
export function AuthProgressCard({
  title,
  detail,
  steps,
  sx,
}: AuthProgressCardProps) {
  return (
    <div sx={[styles.root, sx]} role="status" aria-live="polite">
      <div sx={styles.header}>
        <div sx={styles.iconBox}>
          <Spinner size="md" />
        </div>
        <div sx={styles.body}>
          <div sx={styles.title}>{title}</div>
          <div sx={styles.detail}>{detail}</div>
          <div sx={styles.progress}>
            <ProgressBar indeterminate />
          </div>
          <div sx={styles.steps}>
            {steps.map((step, index) => {
              const active = index === 0
              return (
                <div
                  key={step}
                  sx={[
                    styles.step,
                    active ? styles.activeLabel : styles.pendingLabel,
                  ]}
                >
                  <div
                    sx={[
                      styles.stepIconBox,
                      active ? styles.activeIcon : styles.pendingIcon,
                    ]}
                  >
                    {active ? (
                      <LuKeyRound sx={styles.stepIcon} aria-hidden="true" />
                    ) : index === steps.length - 1 ? (
                      <LuCheck sx={styles.stepIcon} aria-hidden="true" />
                    ) : (
                      <LuShield sx={styles.stepIcon} aria-hidden="true" />
                    )}
                  </div>
                  <span>{step}</span>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}
