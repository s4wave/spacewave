import { useCallback, useEffect, useState } from 'react'
import {
  LuArrowLeft,
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
  LuLoader,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule9: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule10: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule11: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule12: {
    fontSize: '.55rem',
    lineHeight: 1.5,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    letterSpacing: 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    textTransform: 'uppercase',
  },
  rule13: {
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule14: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule15: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule16: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule17: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule18: {
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
    flex: 1,
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule19: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule20: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'right',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule21: { marginTop: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule22: {
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule23: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  rule24: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule26: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule27: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule28: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule29: { minWidth: 0, flex: 1 },
  rule30: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule31: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule32: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule33: {
    position: 'relative',
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rule34: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule35: {
    position: 'absolute',
    right: `calc(${themeLayout['--spacing']} * -1)`,
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    color: themeColors['--color-brand'],
  },
  rule36: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
  },
  rule37: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule38: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule39: { color: themeColors['--color-brand'] },
  rule40: { color: themeColors['--color-destructive'] },
  rule41: { color: themeColors['--color-foreground-alt'] },
  rule42: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule43: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule44: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule45: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule46: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule47: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule48: {
    position: 'relative',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 64)`,
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 80%, transparent)`,
  },
  rule49: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule50: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 'var(--radius-xl)',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule51: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule52: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule53: { width: `calc(${themeLayout['--spacing']} * 56)` },
})

// LoadingDebug renders candidate "unified loading" primitives in one gallery.
// All primitives are inline in this file; none are extracted to web/ui/ yet.
// Once the visual treatment is approved, primitives move to web/ui/loading/.
export function LoadingDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <button
          type="button"
          onClick={goBack}
          sx={stylexStyles.rule3}
          aria-label="Back"
        >
          <LuArrowLeft {...stylex.props(stylexStyles.rule4)} />
        </button>
        <span sx={stylexStyles.rule5}>Loading UI Prototype</span>
      </div>

      <div sx={stylexStyles.rule6}>
        <div
          className={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-6 */ 'mx-auto max-w-3xl space-y-6'
          }
        >
          <Intro />
          <SpinnerSection />
          <ProgressBarSection />
          <LoadingCardSection />
          <LoadingInlineSection />
          <LoadingScreenSection />
        </div>
      </div>
    </div>
  )
}

function Intro() {
  return (
    <div sx={stylexStyles.rule7}>
      <p sx={stylexStyles.rule8}>Unified loading primitive gallery</p>
      <p sx={stylexStyles.rule9}>
        Five primitives (Spinner, ProgressBar, LoadingCard, LoadingInline,
        LoadingScreen) rendered in all states and sizes. Every surface uses the
        modern token set (brand / foreground / background-card / destructive)
        and the glass card stack (border-foreground/6, bg-background-card/30,
        backdrop-blur-sm). Skeleton is intentionally omitted: research shows
        skeleton placeholders can amplify perceived lag on fast loads. The
        current production components (LoadingSpinner, LoadingOverlay,
        LoadingCard, progress.tsx, LoadingScreen) get rewritten to match once
        the treatment is approved; skeleton.tsx and its dormant shimmer keyframe
        get removed.
      </p>
    </div>
  )
}

// -----------------------------------------------------------------------
// Section wrapper
// -----------------------------------------------------------------------

interface SectionProps {
  title: string
  description: string
  children: React.ReactNode
}

function Section({ title, description, children }: SectionProps) {
  return (
    <section
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
      }
    >
      <div>
        <h2 sx={stylexStyles.rule5}>{title}</h2>
        <p sx={stylexStyles.rule10}>{description}</p>
      </div>
      <div sx={stylexStyles.rule11}>{children}</div>
    </section>
  )
}

function Label({ children }: { children: React.ReactNode }) {
  return <div sx={stylexStyles.rule12}>{children}</div>
}

// -----------------------------------------------------------------------
// 1. Spinner
// -----------------------------------------------------------------------

type SpinnerSize = 'sm' | 'md' | 'lg' | 'xl'

const spinnerSizes: Record<SpinnerSize, string> = {
  sm: 'size-3.5',
  md: 'size-4',
  lg: 'size-6',
  xl: 'size-8',
}

// Spinner inherits text color from parent so container state colors apply.
function Spinner({
  size = 'md',
  variant = 'default',
  className,
}: {
  size?: SpinnerSize
  variant?: 'default' | 'brand' | 'muted' | 'destructive' | 'success'
  className?: string
}) {
  const variantCls =
    variant === 'brand'
      ? 'text-brand'
      : variant === 'destructive'
        ? 'text-destructive'
        : variant === 'muted'
          ? 'text-foreground-alt'
          : variant === 'success'
            ? 'text-success'
            : ''
  return (
    <LuLoader
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'animate-spin',
          spinnerSizes[size],
          variantCls,
          className,
        )
      }
      aria-hidden="true"
    />
  )
}

function SpinnerSection() {
  return (
    <Section
      title="Spinner"
      description="Atomic loading indicator. Inherits text color from parent; sized h-3.5 / h-4 / h-6 / h-8. Replaces LoadingSpinner and ~20 inline LuLoader animate-spin sites."
    >
      <div sx={stylexStyles.rule13}>
        {(['sm', 'md', 'lg', 'xl'] as SpinnerSize[]).map((size) => (
          <div key={size} sx={stylexStyles.rule14}>
            <div sx={stylexStyles.rule15}>
              <Spinner size={size} variant="brand" />
            </div>
            <Label>{size}</Label>
          </div>
        ))}
      </div>
      <div sx={stylexStyles.rule16}>
        {(
          [
            { tone: 'brand' },
            { tone: 'muted' },
            { tone: 'destructive' },
            { tone: 'success' },
          ] as const
        ).map(({ tone }) => (
          <div key={tone} sx={stylexStyles.rule14}>
            <div sx={stylexStyles.rule15}>
              <Spinner
                size="lg"
                variant={
                  tone === 'brand'
                    ? 'brand'
                    : tone === 'muted'
                      ? 'muted'
                      : tone === 'destructive'
                        ? 'destructive'
                        : 'success'
                }
              />
            </div>
            <Label>{tone}</Label>
          </div>
        ))}
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 2. ProgressBar
// -----------------------------------------------------------------------

function ProgressBar({
  value,
  indeterminate,
  rate,
}: {
  value?: number
  indeterminate?: boolean
  rate?: string
}) {
  const pct =
    value === undefined ? 0 : Math.max(0, Math.min(100, Math.round(value)))
  return (
    <div sx={stylexStyles.rule17}>
      <div sx={stylexStyles.rule18}>
        {indeterminate ? (
          <div
            className={
              /* TODO(stylex): global or unknown class: animate-progress-indeterminate */ 'bg-brand animate-progress-indeterminate absolute inset-y-0 w-1/3 rounded-full'
            }
          />
        ) : (
          <div
            className={
              /* TODO(stylex): global or unknown class: progress-width */ 'bg-brand progress-width progress-width-transition h-full rounded-full'
            }
            style={{ '--progress-width': `${pct}%` }}
          />
        )}
      </div>
      {rate ? (
        <span sx={stylexStyles.rule19}>{rate}</span>
      ) : !indeterminate ? (
        <span sx={stylexStyles.rule20}>{pct}%</span>
      ) : null}
    </div>
  )
}

function ProgressBarSection() {
  // Animate a demo determinate bar so motion is visible.
  const [value, setValue] = useState(12)
  useEffect(() => {
    const id = setInterval(() => {
      setValue((v) => (v >= 100 ? 0 : v + 3))
    }, 300)
    return () => clearInterval(id)
  }, [])

  return (
    <Section
      title="ProgressBar"
      description="Determinate (with percent or rate label) and indeterminate variants. Uses bg-brand (fixes today's bg-primary drift). Height h-1.5; full-width; tabular-nums labels."
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4'
        }
      >
        <div>
          <Label>Determinate (0 - 100, animated)</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar value={value} />
          </div>
        </div>
        <div>
          <Label>Determinate + rate label</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar value={62} rate="1.5 MiB/s" />
          </div>
        </div>
        <div>
          <Label>Indeterminate</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar indeterminate />
          </div>
        </div>
        <div>
          <Label>Indeterminate + rate label</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar indeterminate rate="Uploading" />
          </div>
        </div>
        <div sx={stylexStyles.rule22}>
          {[10, 35, 65, 92].map((v) => (
            <div key={v}>
              <ProgressBar value={v} />
            </div>
          ))}
        </div>
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 3. LoadingCard
// -----------------------------------------------------------------------

type LoadingState = 'loading' | 'active' | 'synced' | 'error'

interface LoadingView {
  state: LoadingState
  title: string
  detail?: string
  progress?: number
  rate?: { up?: string; down?: string }
  lastActivity?: string
  error?: string
  onRetry?: () => void
  onCancel?: () => void
}

const loadingTileTone: Record<LoadingState, string> = {
  loading: 'bg-foreground/5 text-foreground-alt',
  active: 'bg-brand/10 text-brand',
  synced: 'bg-foreground/5 text-brand',
  error: 'bg-destructive/10 text-destructive',
}

// LoadingCardProgress renders the progress bar, or the transfer rates when no
// progress is known.
function LoadingCardProgress({ view }: { view: LoadingView }) {
  if (view.progress !== undefined) {
    return (
      <div sx={stylexStyles.rule23}>
        <ProgressBar
          value={view.progress * 100}
          rate={view.rate?.down ?? view.rate?.up}
        />
      </div>
    )
  }
  if (!view.rate) return null

  return (
    <div sx={stylexStyles.rule24}>
      <RatePill label="Up" value={view.rate.up ?? '0 B/s'} />
      <RatePill label="Down" value={view.rate.down ?? '0 B/s'} />
    </div>
  )
}

// LoadingCardActions renders the retry and cancel buttons that the view offers.
function LoadingCardActions({ view }: { view: LoadingView }) {
  if (!view.onRetry && !view.onCancel) return null

  return (
    <div sx={stylexStyles.rule25}>
      {view.onRetry ? (
        <button type="button" onClick={view.onRetry} sx={stylexStyles.rule26}>
          Retry
        </button>
      ) : null}
      {view.onCancel ? (
        <button type="button" onClick={view.onCancel} sx={stylexStyles.rule27}>
          Cancel
        </button>
      ) : null}
    </div>
  )
}

function LoadingCard({ view }: { view: LoadingView }) {
  return (
    <div sx={stylexStyles.rule7}>
      <div sx={stylexStyles.rule28}>
        <div
          className={
            /* TODO(stylex): dynamic class or forwarded className */ cn(
              'flex size-8 shrink-0 items-center justify-center rounded-md',
              loadingTileTone[view.state],
            )
          }
        >
          <LoadingCardIcon state={view.state} />
        </div>
        <div sx={stylexStyles.rule29}>
          <div sx={stylexStyles.rule8}>{view.title}</div>
          {view.detail ? (
            <div sx={stylexStyles.rule30}>{view.detail}</div>
          ) : null}
          <LoadingCardProgress view={view} />
          {view.lastActivity ? (
            <div sx={stylexStyles.rule31}>{view.lastActivity}</div>
          ) : null}
          {view.error ? <div sx={stylexStyles.rule32}>{view.error}</div> : null}
          <LoadingCardActions view={view} />
        </div>
      </div>
    </div>
  )
}

function LoadingCardIcon({ state }: { state: LoadingState }) {
  if (state === 'error') {
    return (
      <LuCircleAlert {...stylex.props(stylexStyles.rule4)} aria-hidden="true" />
    )
  }
  if (state === 'synced') {
    return (
      <span sx={stylexStyles.rule33} aria-hidden="true">
        <LuCloud {...stylex.props(stylexStyles.rule34)} />
        <LuCircleCheck {...stylex.props(stylexStyles.rule35)} />
      </span>
    )
  }
  return <Spinner size="md" />
}

function RatePill({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles.rule36}>
      <div sx={stylexStyles.rule12}>{label}</div>
      <div sx={stylexStyles.rule37}>{value}</div>
    </div>
  )
}

function LoadingCardSection() {
  const examples: Array<{ label: string; view: LoadingView }> = [
    {
      label: 'loading (awaiting first status)',
      view: {
        state: 'loading',
        title: 'Checking sync status',
        detail: 'Waiting for the session sync watcher.',
      },
    },
    {
      label: 'active (working, with rate pills)',
      view: {
        state: 'active',
        title: 'Uploading changes',
        detail: 'Sending recent edits to the cloud.',
        rate: { up: '1.5 MiB/s', down: '24 KiB/s' },
        lastActivity: 'Last activity 10:42 PM',
      },
    },
    {
      label: 'active + determinate progress',
      view: {
        state: 'active',
        title: 'Cloning repository',
        detail: 'Receiving objects from origin.',
        progress: 0.62,
        rate: { down: '4.8 MiB/s' },
      },
    },
    {
      label: 'active + indeterminate (object load)',
      view: {
        state: 'active',
        title: 'Loading space',
        detail: 'Resolving root, path, stat, readdir (4 stages).',
      },
    },
    {
      label: 'synced (idle, caught up)',
      view: {
        state: 'synced',
        title: 'Synced',
        detail: 'All sync work complete.',
        rate: { up: '0 B/s', down: '0 B/s' },
        lastActivity: 'Last activity 5 min ago',
      },
    },
    {
      label: 'error (transport down)',
      view: {
        state: 'error',
        title: 'Sync needs attention',
        detail: 'Reconnect to finish uploading changes.',
        error: 'Transport error: WebSocket closed (code 1006).',
        onRetry: () => {},
        onCancel: () => {},
      },
    },
  ]

  return (
    <Section
      title="LoadingCard"
      description="The primary loading surface. Glass card with size-8 icon box + title + detail. Four states. Optionally renders a ProgressBar, rate pills, last-activity footer, error box, and retry/cancel buttons. Lifts the shape from SessionSyncStatusSummary."
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
        }
      >
        {examples.map(({ label, view }) => (
          <div key={label}>
            <Label>{label}</Label>
            <div sx={stylexStyles.rule21}>
              <LoadingCard view={view} />
            </div>
          </div>
        ))}
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 4. LoadingInline
// -----------------------------------------------------------------------

function LoadingInline({
  label,
  size = 'sm',
  tone = 'muted',
}: {
  label: string
  size?: SpinnerSize
  tone?: 'brand' | 'muted' | 'destructive'
}) {
  return (
    <span
      sx={[
        stylexStyles.rule38,
        tone === 'brand'
          ? stylexStyles.rule39
          : tone === 'destructive'
            ? stylexStyles.rule40
            : stylexStyles.rule41,
      ]}
    >
      <Spinner size={size} />
      <span sx={stylexStyles.rule42}>{label}</span>
    </span>
  )
}

function LoadingInlineSection() {
  return (
    <Section
      title="LoadingInline"
      description="Spinner + one-line label. Replaces ~15 ad-hoc <LuLoader/> + text sites (button labels, row pending states, single-line loaders)."
    >
      <div
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ 'space-y-3'
        }
      >
        <div>
          <Label>In place of bare text</Label>
          <div sx={stylexStyles.rule43}>
            <LoadingInline label="Loading messages..." />
          </div>
        </div>
        <div>
          <Label>As a button label (pending action)</Label>
          <div sx={stylexStyles.rule44}>
            <button type="button" sx={stylexStyles.rule45} disabled>
              <Spinner size="sm" variant="brand" />
              <span>Creating…</span>
            </button>
            <button type="button" sx={stylexStyles.rule46} disabled>
              <Spinner size="sm" />
              <span>Retrying…</span>
            </button>
          </div>
        </div>
        <div>
          <Label>Tones</Label>
          <div sx={stylexStyles.rule47}>
            <LoadingInline label="Muted" tone="muted" />
            <LoadingInline label="Brand" tone="brand" />
            <LoadingInline label="Destructive" tone="destructive" />
          </div>
        </div>
      </div>
    </Section>
  )
}

// -----------------------------------------------------------------------
// 5. LoadingScreen (mini preview)
// -----------------------------------------------------------------------

function LoadingScreenSection() {
  const [progress, setProgress] = useState(0)
  useEffect(() => {
    const id = setInterval(() => {
      setProgress((p) => (p >= 100 ? 0 : p + 2))
    }, 200)
    return () => clearInterval(id)
  }, [])

  return (
    <Section
      title="LoadingScreen (mini preview)"
      description="Full-screen boot state. The real component keeps its animated logo + shine border; only the title / detail / progress treatment gets refreshed to match the other primitives. Mini-preview here to avoid taking over the viewport."
    >
      <div sx={stylexStyles.rule48}>
        <div sx={stylexStyles.rule49}>
          <div sx={stylexStyles.rule50}>
            <Spinner size="xl" variant="brand" />
          </div>
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-1 */ 'space-y-1 text-center'
            }
          >
            <div sx={stylexStyles.rule51}>Loading Spacewave</div>
            <div sx={stylexStyles.rule52}>Setting up your session…</div>
          </div>
          <div sx={stylexStyles.rule53}>
            <ProgressBar value={progress} />
          </div>
        </div>
      </div>
    </Section>
  )
}
