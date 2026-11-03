import { LuRefreshCw } from '@s4wave/web/images/icons.js'

import { useRenderDelay } from '@s4wave/app/loading/useRenderDelay.js'

import { BackButton } from '@s4wave/web/ui/BackButton.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import {
  spaceMountStageIndex,
  spaceMountStages,
  type SpaceMountStage,
} from './spaceMountStage.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  motion as themeMotion,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    justifyContent: 'center',
  },
  rule2: {
    display: 'flex',
    alignItems: 'flex-start',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    position: 'relative',
    width: `calc(${themeLayout['--spacing']} * 2)`,
    height: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '3.40282e38px',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule5: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
  rule6: { backgroundColor: themeColors['--color-brand'] },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
  },
  rule8: {
    position: 'absolute',
    inset: '-6px',
    animationName: {
      default: 'ping',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationDuration: {
      default: '1s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationTimingFunction: {
      default: 'cubic-bezier(0, 0, .2, 1)',
      '@media (prefers-reduced-motion: reduce)': 'ease',
    },
    animationIterationCount: {
      default: 'infinite',
      '@media (prefers-reduced-motion: reduce)': 1,
    },
    animationDirection: {
      default: 'normal',
      '@media (prefers-reduced-motion: reduce)': 'normal',
    },
    animationPlayState: {
      default: 'running',
      '@media (prefers-reduced-motion: reduce)': 'running',
    },
    animationDelay: {
      default: '0s',
      '@media (prefers-reduced-motion: reduce)': '0s',
    },
    animationFillMode: {
      default: 'none',
      '@media (prefers-reduced-motion: reduce)': 'none',
    },
    animationTimeline: {
      default: 'auto',
      '@media (prefers-reduced-motion: reduce)': 'auto',
    },
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
  },
  rule9: {
    fontSize: '.6rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    textTransform: 'uppercase',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 55%, transparent)`,
  },
  rule11: { color: themeColors['--color-foreground'] },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 35%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

interface SpaceMountingScreenProps {
  // stage is the current phase of the mount used to drive the stepper.
  stage: SpaceMountStage
  // detail is the live status line shown under the title. Updates as the
  // backing watch advances through stages.
  detail: string
  // title overrides the default "Opening your space" status.
  title?: string
  // onBack renders a floating Back button in the top-left when provided.
  onBack?: () => void
  // onRetry renders a Retry button below the stepper, gated on a short
  // delay so fast loads never flash a Retry CTA.
  onRetry?: () => void
}

const RETRY_DELAY_MS = 5_000

// SpaceMountingScreen presents watched mount progress and delayed recovery
// actions using the same composition as browser and quickstart startup.
export function SpaceMountingScreen({
  stage,
  detail,
  title = 'Opening your space',
  onBack,
  onRetry,
}: SpaceMountingScreenProps) {
  const allowRetry = useRenderDelay(RETRY_DELAY_MS)
  return (
    <LoadingScreen
      view={{ state: 'active', title, detail }}
      topLeftSlot={
        onBack ? (
          <BackButton floating onClick={onBack}>
            Back
          </BackButton>
        ) : undefined
      }
      footer={
        onRetry && allowRetry ? (
          <div sx={stylexStyles2.rule1}>
            <DashboardButton
              icon={<LuRefreshCw sx={stylexStyles.rule1} />}
              onClick={onRetry}
            >
              Retry
            </DashboardButton>
          </div>
        ) : null
      }
    >
      <SpaceMountStepper current={stage} />
    </LoadingScreen>
  )
}

// SpaceMountStepper presents the four watched mount phases.
function SpaceMountStepper({ current }: { current: SpaceMountStage }) {
  const currentIndex = spaceMountStageIndex(current)
  return (
    <div sx={stylexStyles2.rule2}>
      {spaceMountStages.map((entry, i) => {
        const isComplete = i < currentIndex
        const isActive = i === currentIndex
        return (
          <div key={entry.id} sx={stylexStyles2.rule3}>
            <span
              sx={[
                stylexStyles2.rule4,
                !!isComplete && stylexStyles2.rule5,
                !!isActive && stylexStyles2.rule6,
                !!(!isComplete && !isActive) && stylexStyles2.rule7,
              ]}
              aria-hidden="true"
            >
              {isActive ? <span sx={stylexStyles2.rule8} /> : null}
            </span>
            <span
              sx={[
                stylexStyles2.rule9,
                !!isComplete && stylexStyles2.rule10,
                !!isActive && stylexStyles2.rule11,
                !!(!isComplete && !isActive) && stylexStyles2.rule12,
              ]}
            >
              {entry.label}
            </span>
          </div>
        )
      })}
    </div>
  )
}
