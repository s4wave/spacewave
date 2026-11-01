import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
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
} from '../../style/tokens.stylex.js'

import {
  radii as themeRadii2,
  utilityState as themeUtilityState,
  colors as themeColors2,
  layout as themeLayout2,
} from '../../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: themeRadii2['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors2['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors2['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout2['--spacing']} * 3.5)`,
  },
  rule2: { marginTop: `calc(${themeLayout2['--spacing']} * 3)` },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
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
  rule3: { minWidth: 0, flex: 1 },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule6: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  rule7: {
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
  rule8: { color: themeColors['--color-foreground'] },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
  },
  rule11: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule12: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule13: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

interface AuthProgressCardProps {
  className?: never
  title: string
  detail: string
  steps: string[]
  sx?: ComponentStyles
}

// AuthProgressCard shows honest indeterminate progress for auth flows whose
// crypto and provider work does not expose reliable percent completion.
export function AuthProgressCard({
  title,
  detail,
  steps,
  sx,
}: Omit<AuthProgressCardProps, 'className'> & {
  sx?: ComponentStyles
  className?: never
}) {
  return (
    <div sx={[stylexStyles2.rule1, sx]} role="status" aria-live="polite">
      <div sx={stylexStyles.rule1}>
        <div sx={stylexStyles.rule2}>
          <Spinner size="md" />
        </div>
        <div sx={stylexStyles.rule3}>
          <div sx={stylexStyles.rule4}>{title}</div>
          <div sx={stylexStyles.rule5}>{detail}</div>
          <div sx={stylexStyles.rule6}>
            <ProgressBar indeterminate />
          </div>
          <div sx={stylexStyles2.rule2}>
            {steps.map((step, index) => {
              const active = index === 0
              return (
                <div
                  key={step}
                  sx={[
                    stylexStyles.rule7,
                    active ? stylexStyles.rule8 : stylexStyles.rule9,
                  ]}
                >
                  <div
                    sx={[
                      stylexStyles.rule10,
                      active ? stylexStyles.rule11 : stylexStyles.rule12,
                    ]}
                  >
                    {active ? (
                      <LuKeyRound
                        {...stylex.props(stylexStyles.rule13)}
                        aria-hidden="true"
                      />
                    ) : index === steps.length - 1 ? (
                      <LuCheck
                        {...stylex.props(stylexStyles.rule13)}
                        aria-hidden="true"
                      />
                    ) : (
                      <LuShield
                        {...stylex.props(stylexStyles.rule13)}
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
