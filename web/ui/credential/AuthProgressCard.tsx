import type { StyleXStyles } from '@stylexjs/stylex'
import { LuCheck, LuKeyRound, LuShield } from 'react-icons/lu'

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
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  steps: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
  header: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  iconBox: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  body: { minWidth: 0, flex: 1 },
  title: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  detail: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  progress: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  step: {
    marginBlockStart: { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(${themeLayout['--spacing']} * 1.5)`,
    },
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  activeLabel: { color: themeColors['--color-foreground'] },
  pendingLabel: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  stepIconBox: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  activeIcon: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  pendingIcon: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  stepIcon: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
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
                      <LuKeyRound
                        {...stylex.props(styles.stepIcon)}
                        aria-hidden="true"
                      />
                    ) : index === steps.length - 1 ? (
                      <LuCheck
                        {...stylex.props(styles.stepIcon)}
                        aria-hidden="true"
                      />
                    ) : (
                      <LuShield
                        {...stylex.props(styles.stepIcon)}
                        aria-hidden="true"
                      />
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
