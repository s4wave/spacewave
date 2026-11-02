/* eslint-disable react-doctor/no-giant-component */
import { useCallback } from 'react'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
  motion as themeMotion,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    dynamicRangeLimit: 'var(--hdr-dynamic-range)',
    borderColor: 'var(--hdr-border-color)',
    boxShadow: 'var(--hdr-box-shadow)',
    backgroundColor: themeColors['--color-background-deep'],
    backgroundImage: 'none',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 20)`,
    width: `calc(${themeLayout['--spacing']} * 36)`,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    textAlign: 'center',
  },
  rule2: {
    dynamicRangeLimit: 'no-limit',
    backgroundColor: 'var(--hdr-sample-color)',
    filter: 'var(--hdr-sample-filter)',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '9px',
    lineHeight: 1.5,
  },
  rule3: {
    backgroundColor: 'var(--hdr-text-background)',
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    color: 'white',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    color: 'white',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    color: 'white',
    textShadow: '0 0 3px rgb(0 0 0 / 80%)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    dynamicRangeLimit: 'no-limit',
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    dynamicRangeLimit: 'no-limit',
    display: 'flex',
    gap: themeLayout['--spacing'],
  },
  rule9: {
    dynamicRangeLimit: 'no-limit',
    backgroundColor:
      'oklch( var(--hdr-ramp-lightness) var(--hdr-ramp-chroma) var(--hdr-ramp-hue) )',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 12)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'flex-end',
    justifyContent: 'center',
    borderRadius: '.25rem',
    paddingBottom: themeLayout['--spacing'],
    fontSize: '8px',
    lineHeight: 1.5,
  },
  rule10: { color: 'var(--color-black)' },
  rule11: { color: themeColors['--color-white'] },
  rule12: {
    dynamicRangeLimit: 'no-limit',
    borderColor: `var(--hdr-glow-active-border, ${themeColors['--color-ui-outline-active']})`,
    boxShadow: '0 0 10px 2px var(--hdr-glow-active-shadow, transparent)',
    backgroundColor: themeColors['--color-background-primary'],
    backgroundImage: 'none',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 32)`,
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule13: {
    dynamicRangeLimit: 'standard',
    borderColor: themeColors['--color-window-border'],
    boxShadow: '0 2px 8px var(--hdr-shadow-recede, rgb(0 0 0 / 30%))',
    backgroundColor: themeColors['--color-background-primary'],
    backgroundImage: 'none',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 32)`,
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule14: {
    dynamicRangeLimit: 'standard',
    borderColor: themeColors['--color-window-border'],
    backgroundColor: themeColors['--color-background-primary'],
    backgroundImage: 'none',
    backgroundPositionX: '0%',
    backgroundPositionY: '0%',
    backgroundSize: 'auto',
    backgroundRepeat: 'repeat',
    backgroundAttachment: 'scroll',
    backgroundOrigin: 'padding-box',
    backgroundClip: 'border-box',
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 32)`,
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
  rule15: {
    backgroundColor: 'var(--hdr-token-color)',
    height: `calc(${themeLayout['--spacing']} * 6)`,
    width: `calc(${themeLayout['--spacing']} * 12)`,
    borderRadius: '.25rem',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule2: {
    maxWidth: `calc(${themeLayout['--spacing']} * 16)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textAlign: 'center',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-text-primary'],
  },
  rule4: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '9px',
    lineHeight: 1.5,
    color: themeColors['--color-text-muted'],
  },
  rule5: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
  },
  rule6: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule7: { display: 'flex', gap: themeLayout['--spacing'] },
  rule8: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    opacity: 0.6,
  },
  rule9: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule10: {
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    paddingBottom: themeLayout['--spacing'],
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule11: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule12: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule13: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
  rule14: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule15: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: 'var(--text-3xl)',
    lineHeight: 'var(--tw-leading, var(--text-3xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule16: {
    marginBottom: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule17: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule18: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: themeColors['--color-background-card'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule19: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule20: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule21: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule22: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule23: {
    display: 'flex',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: 'var(--radius-xl)',
    backgroundColor: themeColors['--color-background-dark'],
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule24: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-text-primary'],
  },
  rule25: {
    fontSize: '10px',
    lineHeight: 1.5,
    color: themeColors['--color-text-muted'],
  },
  rule26: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule27: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-text-muted'],
    textTransform: 'uppercase',
  },
  rule28: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule29: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 24)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: themeColors['--color-window-border'],
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule30: {
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-text-muted'],
  },
  rule31: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: themeColors['--color-background-card'],
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// Swatch renders a color sample with its token name.
function Swatch({
  label,
  className,
  color,
}: {
  label: string
  className?: string
  color: string
}) {
  return (
    <div sx={stylexStyles.rule1}>
      <div
        className={
          /* TODO(stylex): global or unknown class: hdr-swatch-color */ cn(
            'hdr-swatch-color h-10 w-16 rounded border border-white/10',
            className,
          )
        }
        style={{ '--hdr-swatch-color': color }}
      />
      <span sx={stylexStyles.rule2}>{label}</span>
    </div>
  )
}

// GlowBox renders a box with configurable dynamic-range-limit for HDR testing.
function GlowBox({
  label,
  dynamicRange,
  color,
  glow,
}: {
  label: string
  dynamicRange: 'standard' | 'constrained' | 'no-limit'
  color: string
  glow: string
}) {
  return (
    <div
      sx={stylexStyles2.rule1}
      style={{
        '--hdr-dynamic-range': dynamicRange,
        '--hdr-border-color': color,
        '--hdr-box-shadow': `0 0 12px 2px ${glow}`,
      }}
    >
      <span sx={stylexStyles.rule3}>{label}</span>
      <span sx={stylexStyles.rule4}>{dynamicRange}</span>
    </div>
  )
}

// BrightnessStrip renders a row of boxes at different filter brightness levels.
function BrightnessStrip({ color, label }: { color: string; label: string }) {
  const levels = [1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.5, 3.0]
  return (
    <div sx={stylexStyles.rule5}>
      <span sx={stylexStyles.rule6}>{label}</span>
      <div sx={stylexStyles.rule7}>
        {levels.map((b) => (
          <div
            key={b}
            sx={stylexStyles2.rule2}
            style={{
              '--hdr-sample-color': color,
              '--hdr-sample-filter': `brightness(${b})`,
            }}
          >
            {b}x
          </div>
        ))}
      </div>
    </div>
  )
}

// TextContrast tests text on various backgrounds at different sizes.
function TextContrast({ bg, bgLabel }: { bg: string; bgLabel: string }) {
  return (
    <div sx={stylexStyles2.rule3} style={{ '--hdr-text-background': bg }}>
      <span sx={stylexStyles.rule8}>{bgLabel}</span>
      <span sx={stylexStyles2.rule4}>
        White 10px: The quick brown fox jumps over the lazy dog
      </span>
      <span sx={stylexStyles2.rule4}>
        White 12px: The quick brown fox jumps over the lazy dog
      </span>
      <span sx={stylexStyles2.rule5}>
        White 14px: The quick brown fox jumps over the lazy dog
      </span>
      <span sx={stylexStyles2.rule6}>
        White 12px + shadow: The quick brown fox jumps over the lazy dog
      </span>
    </div>
  )
}

// Section is a simple titled section.
function Section({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section sx={stylexStyles.rule9}>
      <h2 sx={stylexStyles.rule10}>{title}</h2>
      {children}
    </section>
  )
}

export function HDRDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  return (
    <div sx={stylexStyles.rule11}>
      <div sx={stylexStyles.rule12}>
        <button type="button" onClick={goBack} sx={stylexStyles.rule13}>
          <LuArrowLeft {...stylex.props(stylexStyles.rule14)} />
          Back to home
        </button>

        <h1 sx={stylexStyles.rule15}>HDR Debug Lab</h1>
        <p sx={stylexStyles.rule16}>
          Visual test bed for HDR, wide-gamut, and dynamic-range-limit CSS
          features. Compare elements across SDR, P3, and HDR tiers.
        </p>

        <div sx={stylexStyles.rule17}>
          {/* === 1. Display Capability Detection === */}
          <Section title="1. Display Capabilities">
            <div sx={stylexStyles.rule18}>
              <div
                className={
                  /* TODO(stylex): global or unknown class: hdr-debug-gamut-p3 */ 'hdr-debug-gamut-p3 flex items-center gap-2'
                }
              >
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  Wide Gamut (P3)
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-no */ 'cap-no rounded bg-red-500/20 px-2 py-0.5 text-xs font-semibold text-red-400'
                  }
                >
                  NO
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-yes */ 'cap-yes micro-ten hidden rounded bg-green-500/20 px-2 py-0.5 font-semibold text-green-400'
                  }
                >
                  YES
                </span>
                <span sx={stylexStyles.rule6}>@media (color-gamut: p3)</span>
              </div>
              <div
                className={
                  /* TODO(stylex): global or unknown class: hdr-debug-gamut-rec2020 */ 'hdr-debug-gamut-rec2020 flex items-center gap-2'
                }
              >
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  Ultra Wide Gamut (Rec.2020)
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-no */ 'cap-no rounded bg-red-500/20 px-2 py-0.5 text-xs font-semibold text-red-400'
                  }
                >
                  NO
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-yes */ 'cap-yes micro-ten hidden rounded bg-green-500/20 px-2 py-0.5 font-semibold text-green-400'
                  }
                >
                  YES
                </span>
                <span sx={stylexStyles.rule6}>
                  @media (color-gamut: rec2020)
                </span>
              </div>
              <div
                className={
                  /* TODO(stylex): global or unknown class: hdr-debug-dynamic-range */ 'hdr-debug-dynamic-range flex items-center gap-2'
                }
              >
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  HDR Dynamic Range
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-no */ 'cap-no rounded bg-red-500/20 px-2 py-0.5 text-xs font-semibold text-red-400'
                  }
                >
                  NO
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-yes */ 'cap-yes micro-ten hidden rounded bg-green-500/20 px-2 py-0.5 font-semibold text-green-400'
                  }
                >
                  YES
                </span>
                <span sx={stylexStyles.rule6}>
                  @media (dynamic-range: high)
                </span>
              </div>
              <div
                className={
                  /* TODO(stylex): global or unknown class: hdr-debug-dynamic-range-sdr */ 'hdr-debug-dynamic-range-sdr flex items-center gap-2'
                }
              >
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  SDR Dynamic Range
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-no */ 'cap-no rounded bg-red-500/20 px-2 py-0.5 text-xs font-semibold text-red-400'
                  }
                >
                  NO
                </span>
                <span
                  className={
                    /* TODO(stylex): global or unknown class: cap-yes */ 'cap-yes micro-ten hidden rounded bg-green-500/20 px-2 py-0.5 font-semibold text-green-400'
                  }
                >
                  YES
                </span>
                <span sx={stylexStyles.rule6}>
                  @media (dynamic-range: standard)
                </span>
              </div>
            </div>
          </Section>

          {/* === 2. Color Token Comparison (SDR baseline) === */}
          <Section title="2. Color Tokens (current tier active)">
            <p sx={stylexStyles.rule19}>
              These render with whatever tier your display supports (SDR → P3 →
              HDR). Compare with the token values in app.css.
            </p>
            <div sx={stylexStyles.rule20}>
              <div>
                <h3
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
                  }
                >
                  Brand
                </h3>
                <div sx={stylexStyles.rule21}>
                  <Swatch label="brand" color="var(--color-brand)" />
                  <Swatch
                    label="brand-highlight"
                    color="var(--color-brand-highlight)"
                  />
                  <Swatch label="primary" color="var(--color-primary)" />
                  <Swatch
                    label="primary-fg"
                    color="var(--color-primary-foreground)"
                  />
                  <Swatch label="accent" color="var(--color-accent)" />
                  <Swatch label="violet" color="var(--color-violet)" />
                </div>
              </div>
              <div>
                <h3
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
                  }
                >
                  Status
                </h3>
                <div sx={stylexStyles.rule21}>
                  <Swatch label="success" color="var(--color-success)" />
                  <Swatch label="warning" color="var(--color-warning)" />
                  <Swatch label="error" color="var(--color-error)" />
                </div>
              </div>
              <div>
                <h3
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
                  }
                >
                  Console
                </h3>
                <div sx={stylexStyles.rule21}>
                  <Swatch label="output" color="var(--color-console-output)" />
                  <Swatch label="info" color="var(--color-console-info)" />
                  <Swatch label="error" color="var(--color-console-error)" />
                </div>
              </div>
              <div>
                <h3
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
                  }
                >
                  Logo
                </h3>
                <div sx={stylexStyles.rule21}>
                  <Swatch label="blue" color="var(--color-logo-blue)" />
                  <Swatch label="pink" color="var(--color-logo-pink)" />
                  <Swatch label="purple" color="var(--color-logo-purple)" />
                </div>
              </div>
              <div>
                <h3
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
                  }
                >
                  Backgrounds
                </h3>
                <div sx={stylexStyles.rule21}>
                  <Swatch label="bg" color="var(--color-background)" />
                  <Swatch
                    label="bg-dark"
                    color="var(--color-background-dark)"
                  />
                  <Swatch
                    label="bg-primary"
                    color="var(--color-background-primary)"
                  />
                  <Swatch
                    label="bg-secondary"
                    color="var(--color-background-secondary)"
                  />
                  <Swatch
                    label="bg-panel"
                    color="var(--color-background-panel)"
                  />
                  <Swatch
                    label="bg-deep"
                    color="var(--color-background-deep)"
                  />
                </div>
              </div>
              <div>
                <h3
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase'
                  }
                >
                  Borders & UI
                </h3>
                <div sx={stylexStyles.rule21}>
                  <Swatch label="border" color="var(--color-border)" />
                  <Swatch label="ui-outline" color="var(--color-ui-outline)" />
                  <Swatch
                    label="ui-outline-active"
                    color="var(--color-ui-outline-active)"
                  />
                  <Swatch
                    label="window-border"
                    color="var(--color-window-border)"
                  />
                </div>
              </div>
            </div>
          </Section>

          {/* === 3. dynamic-range-limit Comparison === */}
          <Section title="3. dynamic-range-limit Levels">
            <p sx={stylexStyles.rule19}>
              Each box uses the same oklch color but different
              dynamic-range-limit values. On HDR displays, &quot;no-limit&quot;
              should appear visibly brighter.
            </p>
            <div sx={stylexStyles.rule22}>
              <GlowBox
                label="Standard"
                dynamicRange="standard"
                color="oklch(0.80 0.16 10 / 0.5)"
                glow="oklch(0.65 0.12 10 / 0.2)"
              />
              <GlowBox
                label="Constrained"
                dynamicRange="constrained"
                color="oklch(0.80 0.16 10 / 0.5)"
                glow="oklch(0.65 0.12 10 / 0.2)"
              />
              <GlowBox
                label="No Limit"
                dynamicRange="no-limit"
                color="oklch(0.80 0.16 10 / 0.5)"
                glow="oklch(0.65 0.12 10 / 0.2)"
              />
            </div>

            <h3
              className={
                /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mt-4 text-xs font-semibold tracking-wider uppercase'
              }
            >
              Same test with brand color (--color-brand)
            </h3>
            <div sx={stylexStyles.rule22}>
              <GlowBox
                label="Standard"
                dynamicRange="standard"
                color="var(--color-brand)"
                glow="var(--color-brand)"
              />
              <GlowBox
                label="Constrained"
                dynamicRange="constrained"
                color="var(--color-brand)"
                glow="var(--color-brand)"
              />
              <GlowBox
                label="No Limit"
                dynamicRange="no-limit"
                color="var(--color-brand)"
                glow="var(--color-brand)"
              />
            </div>

            <h3
              className={
                /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mt-4 text-xs font-semibold tracking-wider uppercase'
              }
            >
              Deep primary color (--color-primary)
            </h3>
            <div sx={stylexStyles.rule22}>
              <GlowBox
                label="Standard"
                dynamicRange="standard"
                color="var(--color-primary)"
                glow="var(--color-primary)"
              />
              <GlowBox
                label="Constrained"
                dynamicRange="constrained"
                color="var(--color-primary)"
                glow="var(--color-primary)"
              />
              <GlowBox
                label="No Limit"
                dynamicRange="no-limit"
                color="var(--color-primary)"
                glow="var(--color-primary)"
              />
            </div>

            <h3
              className={
                /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary mt-4 text-xs font-semibold tracking-wider uppercase'
              }
            >
              Success / Warning / Error glow
            </h3>
            <div sx={stylexStyles.rule22}>
              <GlowBox
                label="Success"
                dynamicRange="no-limit"
                color="var(--color-success)"
                glow="var(--color-success)"
              />
              <GlowBox
                label="Warning"
                dynamicRange="no-limit"
                color="var(--color-warning)"
                glow="var(--color-warning)"
              />
              <GlowBox
                label="Error"
                dynamicRange="no-limit"
                color="var(--color-error)"
                glow="var(--color-error)"
              />
            </div>
          </Section>

          {/* === 4. Brightness Ramp === */}
          <Section title="4. filter: brightness() Ramp (no-limit)">
            <p sx={stylexStyles.rule19}>
              Tests how filter: brightness() interacts with dynamic-range-limit:
              no-limit. On HDR displays, values above 1.0 should produce visibly
              brighter output.
            </p>
            <div sx={stylexStyles2.rule7}>
              <BrightnessStrip color="var(--color-brand)" label="brand" />
              <BrightnessStrip
                color="var(--color-primary)"
                label="primary (deep)"
              />
              <BrightnessStrip color="var(--color-success)" label="success" />
              <BrightnessStrip color="oklch(1 0 0)" label="white" />
              <BrightnessStrip
                color="var(--color-console-info)"
                label="console-info"
              />
            </div>
          </Section>

          {/* === 5. oklch Lightness Ramp === */}
          <Section title="5. oklch Lightness Ramp (L = 0.0 → 1.5)">
            <p sx={stylexStyles.rule19}>
              Tests oklch lightness values beyond 1.0 under no-limit. On HDR
              displays, L &gt; 1.0 should be brighter than standard white.
            </p>
            <div sx={stylexStyles2.rule8}>
              {[
                0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,
                1.3, 1.4, 1.5,
              ].map((l) => (
                <div
                  key={l}
                  sx={[
                    stylexStyles2.rule9,
                    l > 0.6 ? stylexStyles2.rule10 : stylexStyles2.rule11,
                  ]}
                  style={{
                    '--hdr-ramp-lightness': l,
                    '--hdr-ramp-chroma': 0,
                    '--hdr-ramp-hue': 0,
                  }}
                >
                  {l.toFixed(1)}
                </div>
              ))}
            </div>
            <p sx={stylexStyles.rule19}>
              Same with chroma (hue 10, brand red):
            </p>
            <div sx={stylexStyles2.rule8}>
              {[
                0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,
                1.3, 1.4, 1.5,
              ].map((l) => (
                <div
                  key={l}
                  sx={[
                    stylexStyles2.rule9,
                    l > 0.6 ? stylexStyles2.rule10 : stylexStyles2.rule11,
                  ]}
                  style={{
                    '--hdr-ramp-lightness': l,
                    '--hdr-ramp-chroma': 0.15,
                    '--hdr-ramp-hue': 10,
                  }}
                >
                  {l.toFixed(1)}
                </div>
              ))}
            </div>
            <p sx={stylexStyles.rule19}>
              Same with chroma (hue 153, success green):
            </p>
            <div sx={stylexStyles2.rule8}>
              {[
                0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,
                1.3, 1.4, 1.5,
              ].map((l) => (
                <div
                  key={l}
                  sx={[
                    stylexStyles2.rule9,
                    l > 0.6 ? stylexStyles2.rule10 : stylexStyles2.rule11,
                  ]}
                  style={{
                    '--hdr-ramp-lightness': l,
                    '--hdr-ramp-chroma': 0.22,
                    '--hdr-ramp-hue': 153,
                  }}
                >
                  {l.toFixed(1)}
                </div>
              ))}
            </div>
          </Section>

          {/* === 6. Panel Depth Simulation === */}
          <Section title="6. Panel Depth Hierarchy">
            <p sx={stylexStyles.rule19}>
              Simulates the active/inactive panel glow effect. The
              &quot;Active&quot; panel uses dynamic-range-limit: no-limit with
              the HDR glow tokens. The &quot;Inactive&quot; panel uses
              dynamic-range-limit: standard with a receding shadow.
            </p>
            <div sx={stylexStyles.rule23}>
              <div sx={stylexStyles2.rule12}>
                <span sx={stylexStyles.rule24}>Active Panel</span>
                <span sx={stylexStyles.rule25}>no-limit + glow</span>
              </div>
              <div sx={stylexStyles2.rule13}>
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-sm'
                  }
                >
                  Inactive Panel
                </span>
                <span sx={stylexStyles.rule25}>standard + recede</span>
              </div>
              <div sx={stylexStyles2.rule14}>
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-sm'
                  }
                >
                  Inactive (no shadow)
                </span>
                <span sx={stylexStyles.rule25}>standard, flat</span>
              </div>
            </div>
          </Section>

          {/* === 7. Text Legibility on Colored Backgrounds === */}
          <Section title="7. Text Legibility">
            <p sx={stylexStyles.rule19}>
              White text at different sizes on various backgrounds. Tests
              readability for bar items, status labels, and panel text.
            </p>
            <div sx={stylexStyles.rule9}>
              <TextContrast
                bg="var(--color-brand)"
                bgLabel="--color-brand (L≈0.77)"
              />
              <TextContrast
                bg="var(--color-primary)"
                bgLabel="--color-primary (L≈0.55, deep)"
              />
              <TextContrast
                bg="var(--color-bar-item-selected)"
                bgLabel="--color-bar-item-selected"
              />
              <TextContrast
                bg="var(--color-background-primary)"
                bgLabel="--color-background-primary"
              />
              <TextContrast
                bg="var(--color-background-deep)"
                bgLabel="--color-background-deep"
              />
            </div>
          </Section>

          {/* === 8. HDR Glow Token Preview === */}
          <Section title="8. HDR Glow Tokens">
            <p sx={stylexStyles.rule19}>
              Preview of the --hdr-glow-* tokens. These only resolve inside
              @media (dynamic-range: high). On SDR displays they fall back to
              transparent.
            </p>
            <div sx={stylexStyles.rule21}>
              <Swatch
                label="glow-focus"
                color="var(--hdr-glow-focus, oklch(0.4 0 0 / 0.3))"
              />
              <Swatch
                label="glow-active-border"
                color="var(--hdr-glow-active-border, oklch(0.4 0 0 / 0.3))"
              />
              <Swatch
                label="glow-active-shadow"
                color="var(--hdr-glow-active-shadow, oklch(0.4 0 0 / 0.3))"
              />
              <Swatch
                label="shadow-recede"
                color="var(--hdr-shadow-recede, oklch(0.4 0 0 / 0.3))"
              />
            </div>
          </Section>

          {/* === 9. Side-by-Side Chroma Comparison === */}
          <Section title="9. Chroma Comparison (SDR vs P3 target)">
            <p sx={stylexStyles.rule19}>
              Left column: SDR oklch values. Right column: P3 target values
              (hardcoded). Your display renders whatever it can.
            </p>
            <div sx={stylexStyles.rule26}>
              <div>
                <h3 sx={stylexStyles.rule27}>SDR Chroma</h3>
                <div sx={stylexStyles.rule5}>
                  {[
                    ['brand', 'oklch(0.7717 0.1355 16.5)'],
                    ['primary', 'oklch(0.55 0.20 16.5)'],
                    ['success', 'oklch(0.8771 0.2241 153.23)'],
                    ['warning', 'oklch(0.8405 0.0985 55.41)'],
                    ['error', 'oklch(0.7703 0.1356 20.68)'],
                    ['logo-blue', 'oklch(0.7126 0.1209 226.36)'],
                    ['logo-pink', 'oklch(0.6241 0.1659 336.54)'],
                  ].map(([name, color]) => (
                    <div key={name} sx={stylexStyles.rule28}>
                      <div
                        sx={stylexStyles2.rule15}
                        style={{ '--hdr-token-color': color }}
                      />
                      <span sx={stylexStyles.rule6}>{name}</span>
                    </div>
                  ))}
                </div>
              </div>
              <div>
                <h3 sx={stylexStyles.rule27}>P3 Chroma (boosted)</h3>
                <div sx={stylexStyles.rule5}>
                  {[
                    ['brand', 'oklch(0.7717 0.18 16.5)'],
                    ['primary', 'oklch(0.55 0.26 16.5)'],
                    ['success', 'oklch(0.8771 0.28 153.23)'],
                    ['warning', 'oklch(0.8405 0.14 55.41)'],
                    ['error', 'oklch(0.7104 0.17 18.72)'],
                    ['logo-blue', 'oklch(0.7057 0.12 220.43)'],
                    ['logo-pink', 'oklch(0.5913 0.2 330.41)'],
                  ].map(([name, color]) => (
                    <div key={name} sx={stylexStyles.rule28}>
                      <div
                        sx={stylexStyles2.rule15}
                        style={{ '--hdr-token-color': color }}
                      />
                      <span sx={stylexStyles.rule6}>{name}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </Section>

          {/* === 10. Window Glow Hover Test === */}
          <Section title="10. Window Glow Hover Test">
            <p sx={stylexStyles.rule19}>
              Hover over these boxes to see the HDR window glow effect. On SDR
              displays, the hdr-window-glow class has no effect.
            </p>
            <div sx={stylexStyles.rule23}>
              <div
                className={
                  /* TODO(stylex): global or unknown class: hdr-window-glow */ 'hdr-window-glow border-window-border bg-background-primary flex h-24 flex-1 items-center justify-center rounded-lg border'
                }
              >
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  Hover me (window glow)
                </span>
              </div>
              <div
                className={
                  /* TODO(stylex): global or unknown class: hdr-window-glow */ 'hdr-window-glow border-ui-outline bg-background-secondary flex h-24 flex-1 items-center justify-center rounded-lg border'
                }
              >
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  Hover me (outline glow)
                </span>
              </div>
              <div sx={stylexStyles.rule29}>
                <span
                  className={
                    /* TODO(stylex): global class adds declarations outside the utility compiler: text-text-secondary */ 'text-text-secondary text-xs'
                  }
                >
                  No glow (control)
                </span>
              </div>
            </div>
          </Section>

          {/* === 11. Status Flash Test === */}
          <Section title="11. Status Flash Test">
            <p sx={stylexStyles.rule19}>
              Click the buttons to trigger the HDR flash animation on the
              indicator. On SDR displays, the class has no visual effect.
            </p>
            <StatusFlashDemo />
          </Section>

          <div sx={stylexStyles.rule30}>
            <p>
              <strong>Tips:</strong> To test HDR, use a display that supports it
              (MacBook Pro XDR, external HDR monitor). Set display brightness
              below max; HDR headroom only exists when the display has room to
              go brighter. Chrome 133+ supports dynamic-range-limit.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

// Interactive demo for the status flash animation.
function StatusFlashDemo() {
  return <StatusFlashDemoInner />
}

function StatusFlashDemoInner() {
  const statuses = ['success', 'error', 'pending', 'none'] as const
  const statusColors: Record<string, string> = {
    success: 'var(--color-success)',
    error: 'var(--color-error)',
    pending: 'var(--color-warning)',
    none: 'var(--color-text-muted)',
  }

  return (
    <div sx={stylexStyles.rule31}>
      {statuses.map((status) => (
        <button
          type="button"
          key={status}
          className={
            /* TODO(stylex): global or unknown class: hdr-status-color */ 'hdr-status-color hdr-status-flash cursor-pointer rounded border border-white/10 px-3 py-2 font-mono text-xs font-bold'
          }
          style={{ '--hdr-status-color': statusColors[status] }}
          onClick={(e) => {
            const el = e.currentTarget
            el.removeAttribute('data-status-changed')
            // Force reflow to restart animation
            void el.offsetWidth
            el.setAttribute('data-status-changed', '')
          }}
        >
          {status.toUpperCase()}
        </button>
      ))}
      <span sx={stylexStyles.rule19}>Click to trigger flash</span>
    </div>
  )
}
