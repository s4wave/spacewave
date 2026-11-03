import { keyframeNames } from '../style/tokens.stylex.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import type { SpinnerSize } from '@s4wave/web/ui/loading/Spinner.js'
import { useCallback, useEffect, useState } from 'react'
import {
  LuArrowLeft,
  LuCircleAlert,
  LuCircleCheck,
  LuCloud,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles6 = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    insetBlock: 0,
    backgroundColor: themeColors['--color-brand'],
    position: 'absolute',
    width: '33.3333%',
  },
})

const stylexStyles5 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: { marginInline: 'auto', maxWidth: 'var(--container-3xl)' },
  rule2: { textAlign: 'center' },
})

const childSpacing = stylex.create({
  s6: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 6 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
  },
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    transition: 'width 200ms',
    backgroundColor: themeColors['--color-brand'],
    height: '100%',
    width: 'var(--progress-width)',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule8: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule10: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule11: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: '.55rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule13: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
  },
  rule14: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule15: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule17: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    width: '100%',
  },
  rule18: {
    borderRadius: '3.40282e38px',
    flex: 1,
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule19: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule20: {
    '--tw-numeric-spacing': 'tabular-nums',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'right',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule21: { marginTop: `calc(${themeLayout['--spacing']} * 1.5)` },
  rule22: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
  },
  rule23: { marginTop: `calc(${themeLayout['--spacing']} * 2.5)` },
  rule24: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule25: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule26: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule27: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: themeRadii['--radius-md'],
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule28: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule29: { flex: 1, minWidth: 0 },
  rule30: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule31: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule32: {
    '--tw-leading': 'var(--leading-relaxed)',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule33: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule34: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule35: {
    color: themeColors['--color-brand'],
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * -1)`,
    height: `calc(${themeLayout['--spacing']} * 2.5)`,
    right: `calc(${themeLayout['--spacing']} * -1)`,
    width: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule36: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule37: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-numeric-spacing': 'tabular-nums',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule38: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: 'inline-flex',
  },
  rule39: { color: themeColors['--color-brand'] },
  rule40: { color: themeColors['--color-destructive'] },
  rule41: { color: themeColors['--color-foreground-alt'] },
  rule42: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule43: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule44: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule45: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule46: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule47: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule48: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 80%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 64)`,
  },
  rule49: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
  },
  rule50: {
    borderRadius: 'var(--radius-xl)',
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule51: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    userSelect: 'none',
  },
  rule52: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule53: { width: `calc(${themeLayout['--spacing']} * 56)` },
})

// LoadingDebug renders candidate "unified loading" primitives in one gallery.
// All primitives are inline in this file; none are extracted to web/ui/ yet.
// Once the visual treatment is approved, primitives move to web/ui/loading/.
export function LoadingDebug({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={[stylexStyles.rule1, sx]}>
      <div sx={stylexStyles.rule2}>
        <button
          type="button"
          onClick={goBack}
          sx={stylexStyles.rule3}
          aria-label="Back"
        >
          <LuArrowLeft sx={stylexStyles.rule4} />
        </button>
        <span sx={stylexStyles.rule5}>Loading UI Prototype</span>
      </div>

      <div sx={stylexStyles.rule6}>
        <div sx={stylexStyles3.rule1}>
          <Intro sx={childSpacing.s6} />
          <SpinnerSection sx={childSpacing.s6} />
          <ProgressBarSection sx={childSpacing.s6} />
          <LoadingCardSection sx={childSpacing.s6} />
          <LoadingInlineSection sx={childSpacing.s6} />
          <LoadingScreenSection sx={childSpacing.s6} />
        </div>
      </div>
    </div>
  )
}

function Intro({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <div sx={[stylexStyles.rule7, sx]}>
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
  sx?: stylex.StyleXStyles
}

function Section({ title, description, children, sx }: SectionProps) {
  return (
    <section sx={sx}>
      <div sx={childSpacing.s3}>
        <h2 sx={stylexStyles.rule5}>{title}</h2>
        <p sx={stylexStyles.rule10}>{description}</p>
      </div>
      <div sx={[stylexStyles.rule11, childSpacing.s3]}>{children}</div>
    </section>
  )
}

function Label({ children }: { children: React.ReactNode }) {
  return <div sx={stylexStyles.rule12}>{children}</div>
}

// -----------------------------------------------------------------------
// 1. Spinner
// -----------------------------------------------------------------------

// Spinner uses the shared SVG owner for the loading gallery's size and tone controls.
function SpinnerSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <Section
      sx={sx}
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
          <div sx={[stylexStyles6.rule1, progressAnimation.sweep]} />
        ) : (
          <div
            sx={stylexStyles2.rule1}
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

function ProgressBarSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
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
      sx={sx}
      title="ProgressBar"
      description="Determinate (with percent or rate label) and indeterminate variants. Uses bg-brand (fixes today's bg-primary drift). Height h-1.5; full-width; tabular-nums labels."
    >
      <div>
        <div sx={childSpacing.s4}>
          <Label>Determinate (0 - 100, animated)</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar value={value} />
          </div>
        </div>
        <div sx={childSpacing.s4}>
          <Label>Determinate + rate label</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar value={62} rate="1.5 MiB/s" />
          </div>
        </div>
        <div sx={childSpacing.s4}>
          <Label>Indeterminate</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar indeterminate />
          </div>
        </div>
        <div sx={childSpacing.s4}>
          <Label>Indeterminate + rate label</Label>
          <div sx={stylexStyles.rule21}>
            <ProgressBar indeterminate rate="Uploading" />
          </div>
        </div>
        <div sx={[stylexStyles.rule22, childSpacing.s4]}>
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

const loadingTileTone: Record<LoadingState, stylex.StyleXStyles> = {
  loading: [stylexStyles4.rule1],
  active: [stylexStyles4.rule2],
  synced: [stylexStyles4.rule3],
  error: [stylexStyles4.rule4],
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
        <div sx={[[stylexStyles5.rule1], [loadingTileTone[view.state]]]}>
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
    return <LuCircleAlert sx={stylexStyles.rule4} aria-hidden="true" />
  }
  if (state === 'synced') {
    return (
      <span sx={stylexStyles.rule33} aria-hidden="true">
        <LuCloud sx={stylexStyles.rule34} />
        <LuCircleCheck sx={stylexStyles.rule35} />
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

function LoadingCardSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
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
      sx={sx}
      title="LoadingCard"
      description="The primary loading surface. Glass card with size-8 icon box + title + detail. Four states. Optionally renders a ProgressBar, rate pills, last-activity footer, error box, and retry/cancel buttons. Lifts the shape from SessionSyncStatusSummary."
    >
      <div>
        {examples.map(({ label, view }) => (
          <div sx={childSpacing.s3} key={label}>
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

function LoadingInlineSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  return (
    <Section
      sx={sx}
      title="LoadingInline"
      description="Spinner + one-line label. Replaces ~15 ad-hoc <LuLoader/> + text sites (button labels, row pending states, single-line loaders)."
    >
      <div>
        <div sx={childSpacing.s3}>
          <Label>In place of bare text</Label>
          <div sx={stylexStyles.rule43}>
            <LoadingInline label="Loading messages..." />
          </div>
        </div>
        <div sx={childSpacing.s3}>
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
        <div sx={childSpacing.s3}>
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

function LoadingScreenSection({ sx }: { sx?: stylex.StyleXStyles } = {}) {
  const [progress, setProgress] = useState(0)
  useEffect(() => {
    const id = setInterval(() => {
      setProgress((p) => (p >= 100 ? 0 : p + 2))
    }, 200)
    return () => clearInterval(id)
  }, [])

  return (
    <Section
      sx={sx}
      title="LoadingScreen (mini preview)"
      description="Full-screen boot state. The real component keeps its animated logo + shine border; only the title / detail / progress treatment gets refreshed to match the other primitives. Mini-preview here to avoid taking over the viewport."
    >
      <div sx={stylexStyles.rule48}>
        <div sx={stylexStyles.rule49}>
          <div sx={stylexStyles.rule50}>
            <Spinner size="xl" variant="brand" />
          </div>
          <div sx={stylexStyles3.rule2}>
            <div sx={[stylexStyles.rule51, childSpacing.s1]}>
              Loading Spacewave
            </div>
            <div sx={[stylexStyles.rule52, childSpacing.s1]}>
              Setting up your session…
            </div>
          </div>
          <div sx={stylexStyles.rule53}>
            <ProgressBar value={progress} />
          </div>
        </div>
      </div>
    </Section>
  )
}

const progressAnimation = stylex.create({
  sweep: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1.4s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['progress-indeterminate'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'ease-in-out',
  },
})
