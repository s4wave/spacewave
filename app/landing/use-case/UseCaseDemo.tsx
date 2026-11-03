import { keyframeNames } from '../../../web/style/tokens.stylex.js'
import {
  Suspense,
  useEffect,
  useId,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react'
import { LuPlay, LuRotateCcw, LuX } from '@s4wave/web/images/icons.js'

import { useAppHref } from '@s4wave/app/prerender/StaticContext.js'
import { LoadingScreen } from '@s4wave/web/ui/loading/LoadingScreen.js'

import type { DemoId } from './demos.js'
import type { LiveDemoAppProps } from './LiveDemoApp.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 64rem)': 'row',
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-6xl)',
    width: '100%',
  },
  rule2: {
    '--tw-shadow': '0 25px 50px -12px var(--tw-shadow-color, #00000040)',
    '--tw-shadow-color':
      'color-mix(in oklab, color-mix(in oklab, var(--color-black) 30%, transparent) var(--tw-shadow-alpha), transparent)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    overflow: 'hidden',
    backgroundColor: themeColors['--color-background'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    display: 'flex',
    flexDirection: 'column',
    height: {
      default: `calc(${themeLayout['--spacing']} * 136)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 152)`,
    },
    minWidth: 0,
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-landing']} 60%, transparent)`,
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    borderRadius: '3.40282e38px',
    height: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '2s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['pulse'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'cubic-bezier(.4, 0, .6, 1)',
    backgroundColor: themeColors['--color-brand'],
  },
  rule6: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 25%, transparent)`,
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule8: {
    color: themeColors['--color-foreground-alt'],
    display: { default: 'none', '@container (min-width: 28rem)': 'inline' },
  },
  rule9: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    marginLeft: 'auto',
  },
  rule10: {
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule11: {
    borderRadius: '.25rem',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    display: 'flex',
    textDecorationLine: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule12: { flex: 1, position: 'relative', minHeight: 0 },
  rule13: {
    WebkitUserSelect: 'none',
    opacity: 0.6,
    userSelect: 'none',
    height: '100%',
  },
  rule14: {
    '--tw-backdrop-blur': 'blur(var(--blur-xs))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    inset: 0,
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    position: 'absolute',
    textAlign: 'center',
  },
  rule15: {
    '--tw-duration': '.3s',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitUserSelect: 'none',
    borderRadius: themeRadii['--radius-md'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    backgroundColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
      },
    },
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    color: themeColors['--color-background'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: 'none',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    userSelect: 'none',
  },
  rule16: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    maxWidth: 'var(--container-xs)',
  },
  rule17: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
    flexShrink: { default: null, '@container (min-width: 64rem)': 0 },
    paddingTop: {
      default: null,
      '@container (min-width: 64rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
    width: {
      default: null,
      '@container (min-width: 64rem)': `calc(${themeLayout['--spacing']} * 68)`,
    },
  },
  rule18: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textTransform: 'uppercase',
  },
  rule19: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule20: {
    '--tw-leading': 'var(--leading-relaxed)',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule21: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule22: {
    '--tw-leading': 'var(--leading-relaxed)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// UseCaseDemoProps configures a use-case demo frame.
export interface UseCaseDemoProps {
  demo: DemoId
  // liveApp runs the demo. Only the app routes supply it, so static pages
  // never bundle the app.
  liveApp?: ComponentType<LiveDemoAppProps>
  label: string
  poster: ReactNode
  suggestions: string[]
}

// UseCaseDemo frames a use-case demo. Until the visitor starts it, the frame
// shows a static poster of the seeded Space; the live route mounts the real
// app on an in-memory World in the same frame.
export function UseCaseDemo({
  demo,
  liveApp: LiveApp,
  label,
  poster,
  suggestions,
}: UseCaseDemoProps) {
  const live = LiveApp !== undefined
  const startHref = useAppHref(`/landing/${demo}/live`)
  const stopHref = useAppHref(`/landing/${demo}`)
  const instanceId = useId()
  const [run, setRun] = useState(0)
  const frameRef = useRef<HTMLDivElement>(null)

  // Bring the frame into view when the live route opens.
  useEffect(() => {
    if (live) frameRef.current?.scrollIntoView({ block: 'center' })
  }, [live])

  return (
    <section aria-label={label} sx={stylexStyles2.rule1}>
      <div ref={frameRef} sx={stylexStyles2.rule2}>
        <div sx={stylexStyles2.rule3}>
          <span
            sx={[
              stylexStyles2.rule4,
              live ? stylexStyles2.rule5 : stylexStyles2.rule6,
            ]}
          />
          <span sx={stylexStyles2.rule7}>{label}</span>
          <span sx={stylexStyles2.rule8}>
            {live ? 'Running in memory in this tab' : 'Preview'}
          </span>
          {live && (
            <div sx={stylexStyles2.rule9}>
              <button
                type="button"
                onClick={() => setRun((value) => value + 1)}
                sx={stylexStyles2.rule10}
              >
                <LuRotateCcw sx={stylexStyles.rule1} />
                Reset
              </button>
              <a href={stopHref} sx={stylexStyles2.rule11}>
                <LuX sx={stylexStyles.rule1} />
                Stop
              </a>
            </div>
          )}
        </div>

        {LiveApp ? (
          <Suspense
            fallback={
              <LoadingScreen
                view={{ state: 'loading', title: 'Preparing your workspace' }}
              />
            }
          >
            <LiveApp
              key={run}
              demo={demo}
              appId={`landing:${demo}:${instanceId}:${run}`}
            />
          </Suspense>
        ) : (
          <div sx={stylexStyles2.rule12}>
            <div aria-hidden sx={stylexStyles2.rule13}>
              {poster}
            </div>
            <div sx={stylexStyles2.rule14}>
              <a
                href={startHref}
                data-landing-live-start
                sx={stylexStyles2.rule15}
              >
                <LuPlay sx={stylexStyles.rule2} />
                Try it live
              </a>
              <p sx={stylexStyles2.rule16}>
                Starts a real Spacewave workspace in this tab. It is kept in
                memory and forgotten when you leave.
              </p>
            </div>
          </div>
        )}
      </div>

      <aside sx={stylexStyles2.rule17}>
        <h2 sx={stylexStyles2.rule18}>Try this</h2>
        <ol sx={stylexStyles2.rule19}>
          {suggestions.map((suggestion, index) => (
            <li key={suggestion} sx={stylexStyles2.rule20}>
              <span sx={stylexStyles2.rule21}>{index + 1}</span>
              <span>{suggestion}</span>
            </li>
          ))}
        </ol>
        <p sx={stylexStyles2.rule22}>
          Reset starts the demo over. Nothing here reaches your saved Spaces.
        </p>
      </aside>
    </section>
  )
}
