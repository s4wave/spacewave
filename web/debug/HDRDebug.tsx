import { useCallback } from 'react'
import { LuArrowLeft } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'

// extStyle accepts a style object with extended CSS properties (such as
// dynamicRangeLimit) and returns it as React.CSSProperties. Centralizes the
// single type widening needed for non-standard CSS properties.
function extStyle(s: Record<string, unknown>): React.CSSProperties {
  return s as never
}

// Swatch renders a color sample with its token name.
function Swatch({
  label,
  className,
  style,
}: {
  label: string
  className?: string
  style?: React.CSSProperties
}) {
  return (
    <div className="flex flex-col items-center gap-1">
      <div
        className={cn('h-10 w-16 rounded border border-white/10', className)}
        style={style}
      />
      <span className="text-text-muted max-w-16 truncate text-center font-mono text-[9px]">
        {label}
      </span>
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
      className="flex h-20 w-36 flex-col items-center justify-center rounded-lg border text-center"
      style={extStyle({
        dynamicRangeLimit: dynamicRange,
        borderColor: color,
        boxShadow: `0 0 12px 2px ${glow}`,
        background: 'var(--color-background-deep)',
      })}
    >
      <span className="text-text-primary text-xs font-semibold">{label}</span>
      <span className="text-text-muted font-mono text-[9px]">
        {dynamicRange}
      </span>
    </div>
  )
}

// BrightnessStrip renders a row of boxes at different filter brightness levels.
function BrightnessStrip({ color, label }: { color: string; label: string }) {
  const levels = [1.0, 1.2, 1.4, 1.6, 1.8, 2.0, 2.5, 3.0]
  return (
    <div className="flex flex-col gap-1">
      <span className="text-text-muted font-mono text-[10px]">{label}</span>
      <div className="flex gap-1">
        {levels.map((b) => (
          <div
            key={b}
            className="flex h-8 w-12 items-center justify-center rounded font-mono text-[9px]"
            style={extStyle({
              dynamicRangeLimit: 'no-limit',
              backgroundColor: color,
              filter: `brightness(${b})`,
            })}
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
    <div
      className="flex flex-col gap-2 rounded-lg p-3"
      style={{ backgroundColor: bg }}
    >
      <span className="font-mono text-[9px] opacity-60">{bgLabel}</span>
      <span className="text-[10px]" style={{ color: 'white' }}>
        White 10px — The quick brown fox jumps over the lazy dog
      </span>
      <span className="text-xs" style={{ color: 'white' }}>
        White 12px — The quick brown fox jumps over the lazy dog
      </span>
      <span className="text-sm" style={{ color: 'white' }}>
        White 14px — The quick brown fox jumps over the lazy dog
      </span>
      <span
        className="text-xs"
        style={{ color: 'white', textShadow: '0 0 3px rgba(0,0,0,0.8)' }}
      >
        White 12px + shadow — The quick brown fox jumps over the lazy dog
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
    <section className="flex flex-col gap-3">
      <h2 className="text-foreground border-b border-white/10 pb-1 text-lg font-semibold">
        {title}
      </h2>
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
    <div className="bg-background @container flex w-full flex-1 flex-col overflow-y-auto">
      {/* Inline styles for CSS-only capability detection */}
      <style>{`
        @media (color-gamut: p3) {
          .hdr-debug-gamut-p3 .cap-no { display: none !important; }
          .hdr-debug-gamut-p3 .cap-yes { display: inline !important; }
        }
        @media (color-gamut: rec2020) {
          .hdr-debug-gamut-rec2020 .cap-no { display: none !important; }
          .hdr-debug-gamut-rec2020 .cap-yes { display: inline !important; }
        }
        @media (dynamic-range: high) {
          .hdr-debug-dynamic-range .cap-no { display: none !important; }
          .hdr-debug-dynamic-range .cap-yes { display: inline !important; }
        }
        @media (dynamic-range: standard) {
          .hdr-debug-dynamic-range-sdr .cap-no { display: none !important; }
          .hdr-debug-dynamic-range-sdr .cap-yes { display: inline !important; }
        }

        .hdr-debug-drl-test {
          dynamic-range-limit: no-limit;
        }

        /* Overbright gradient ramp */
        .hdr-debug-overbright-ramp {
          dynamic-range-limit: no-limit;
        }
      `}</style>

      <div className="mx-auto w-full max-w-5xl px-4 py-6 @lg:px-8">
        <button
          onClick={goBack}
          className="text-foreground-alt hover:text-foreground mb-6 flex cursor-pointer items-center gap-2 transition-colors"
        >
          <LuArrowLeft className="h-4 w-4" />
          Back to home
        </button>

        <h1 className="text-foreground mb-2 text-3xl font-bold">
          HDR Debug Lab
        </h1>
        <p className="text-foreground-alt mb-8 text-sm">
          Visual test bed for HDR, wide-gamut, and dynamic-range-limit CSS
          features. Compare elements across SDR, P3, and HDR tiers.
        </p>

        <div className="flex flex-col gap-10">
          {/* === 1. Display Capability Detection === */}
          <Section title="1. Display Capabilities">
            <div className="bg-background-card flex flex-col gap-2 rounded-lg p-4">
              <div className="hdr-debug-gamut-p3 flex items-center gap-2">
                <span className="text-text-secondary text-xs">
                  Wide Gamut (P3)
                </span>
                <span className="cap-no rounded bg-red-500/20 px-2 py-0.5 text-[10px] font-semibold text-red-400">
                  NO
                </span>
                <span className="cap-yes hidden rounded bg-green-500/20 px-2 py-0.5 text-[10px] font-semibold text-green-400">
                  YES
                </span>
                <span className="text-text-muted font-mono text-[9px]">
                  @media (color-gamut: p3)
                </span>
              </div>
              <div className="hdr-debug-gamut-rec2020 flex items-center gap-2">
                <span className="text-text-secondary text-xs">
                  Ultra Wide Gamut (Rec.2020)
                </span>
                <span className="cap-no rounded bg-red-500/20 px-2 py-0.5 text-[10px] font-semibold text-red-400">
                  NO
                </span>
                <span className="cap-yes hidden rounded bg-green-500/20 px-2 py-0.5 text-[10px] font-semibold text-green-400">
                  YES
                </span>
                <span className="text-text-muted font-mono text-[9px]">
                  @media (color-gamut: rec2020)
                </span>
              </div>
              <div className="hdr-debug-dynamic-range flex items-center gap-2">
                <span className="text-text-secondary text-xs">
                  HDR Dynamic Range
                </span>
                <span className="cap-no rounded bg-red-500/20 px-2 py-0.5 text-[10px] font-semibold text-red-400">
                  NO
                </span>
                <span className="cap-yes hidden rounded bg-green-500/20 px-2 py-0.5 text-[10px] font-semibold text-green-400">
                  YES
                </span>
                <span className="text-text-muted font-mono text-[9px]">
                  @media (dynamic-range: high)
                </span>
              </div>
              <div className="hdr-debug-dynamic-range-sdr flex items-center gap-2">
                <span className="text-text-secondary text-xs">
                  SDR Dynamic Range
                </span>
                <span className="cap-no rounded bg-red-500/20 px-2 py-0.5 text-[10px] font-semibold text-red-400">
                  NO
                </span>
                <span className="cap-yes hidden rounded bg-green-500/20 px-2 py-0.5 text-[10px] font-semibold text-green-400">
                  YES
                </span>
                <span className="text-text-muted font-mono text-[9px]">
                  @media (dynamic-range: standard)
                </span>
              </div>
            </div>
          </Section>

          {/* === 2. Color Token Comparison (SDR baseline) === */}
          <Section title="2. Color Tokens (current tier active)">
            <p className="text-text-muted text-xs">
              These render with whatever tier your display supports (SDR → P3 →
              HDR). Compare with the token values in app.css.
            </p>
            <div className="flex flex-col gap-4">
              <div>
                <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
                  Brand
                </h3>
                <div className="flex flex-wrap gap-3">
                  <Swatch
                    label="brand"
                    style={{ backgroundColor: 'var(--color-brand)' }}
                  />
                  <Swatch
                    label="brand-highlight"
                    style={{ backgroundColor: 'var(--color-brand-highlight)' }}
                  />
                  <Swatch
                    label="primary"
                    style={{ backgroundColor: 'var(--color-primary)' }}
                  />
                  <Swatch
                    label="primary-fg"
                    style={{
                      backgroundColor: 'var(--color-primary-foreground)',
                    }}
                  />
                  <Swatch
                    label="accent"
                    style={{ backgroundColor: 'var(--color-accent)' }}
                  />
                  <Swatch
                    label="violet"
                    style={{ backgroundColor: 'var(--color-violet)' }}
                  />
                </div>
              </div>
              <div>
                <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
                  Status
                </h3>
                <div className="flex flex-wrap gap-3">
                  <Swatch
                    label="success"
                    style={{ backgroundColor: 'var(--color-success)' }}
                  />
                  <Swatch
                    label="warning"
                    style={{ backgroundColor: 'var(--color-warning)' }}
                  />
                  <Swatch
                    label="error"
                    style={{ backgroundColor: 'var(--color-error)' }}
                  />
                </div>
              </div>
              <div>
                <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
                  Console
                </h3>
                <div className="flex flex-wrap gap-3">
                  <Swatch
                    label="output"
                    style={{ backgroundColor: 'var(--color-console-output)' }}
                  />
                  <Swatch
                    label="info"
                    style={{ backgroundColor: 'var(--color-console-info)' }}
                  />
                  <Swatch
                    label="error"
                    style={{ backgroundColor: 'var(--color-console-error)' }}
                  />
                </div>
              </div>
              <div>
                <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
                  Logo
                </h3>
                <div className="flex flex-wrap gap-3">
                  <Swatch
                    label="blue"
                    style={{ backgroundColor: 'var(--color-logo-blue)' }}
                  />
                  <Swatch
                    label="pink"
                    style={{ backgroundColor: 'var(--color-logo-pink)' }}
                  />
                  <Swatch
                    label="purple"
                    style={{ backgroundColor: 'var(--color-logo-purple)' }}
                  />
                </div>
              </div>
              <div>
                <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
                  Backgrounds
                </h3>
                <div className="flex flex-wrap gap-3">
                  <Swatch
                    label="bg"
                    style={{ backgroundColor: 'var(--color-background)' }}
                  />
                  <Swatch
                    label="bg-dark"
                    style={{ backgroundColor: 'var(--color-background-dark)' }}
                  />
                  <Swatch
                    label="bg-primary"
                    style={{
                      backgroundColor: 'var(--color-background-primary)',
                    }}
                  />
                  <Swatch
                    label="bg-secondary"
                    style={{
                      backgroundColor: 'var(--color-background-secondary)',
                    }}
                  />
                  <Swatch
                    label="bg-panel"
                    style={{
                      backgroundColor: 'var(--color-background-panel)',
                    }}
                  />
                  <Swatch
                    label="bg-deep"
                    style={{
                      backgroundColor: 'var(--color-background-deep)',
                    }}
                  />
                </div>
              </div>
              <div>
                <h3 className="text-text-secondary mb-2 text-xs font-semibold tracking-wider uppercase">
                  Borders & UI
                </h3>
                <div className="flex flex-wrap gap-3">
                  <Swatch
                    label="border"
                    style={{ backgroundColor: 'var(--color-border)' }}
                  />
                  <Swatch
                    label="ui-outline"
                    style={{ backgroundColor: 'var(--color-ui-outline)' }}
                  />
                  <Swatch
                    label="ui-outline-active"
                    style={{
                      backgroundColor: 'var(--color-ui-outline-active)',
                    }}
                  />
                  <Swatch
                    label="window-border"
                    style={{ backgroundColor: 'var(--color-window-border)' }}
                  />
                </div>
              </div>
            </div>
          </Section>

          {/* === 3. dynamic-range-limit Comparison === */}
          <Section title="3. dynamic-range-limit Levels">
            <p className="text-text-muted text-xs">
              Each box uses the same oklch color but different
              dynamic-range-limit values. On HDR displays, &quot;no-limit&quot;
              should appear visibly brighter.
            </p>
            <div className="flex flex-wrap gap-4">
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

            <h3 className="text-text-secondary mt-4 text-xs font-semibold tracking-wider uppercase">
              Same test with brand color (--color-brand)
            </h3>
            <div className="flex flex-wrap gap-4">
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

            <h3 className="text-text-secondary mt-4 text-xs font-semibold tracking-wider uppercase">
              Deep primary color (--color-primary)
            </h3>
            <div className="flex flex-wrap gap-4">
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

            <h3 className="text-text-secondary mt-4 text-xs font-semibold tracking-wider uppercase">
              Success / Warning / Error glow
            </h3>
            <div className="flex flex-wrap gap-4">
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
            <p className="text-text-muted text-xs">
              Tests how filter: brightness() interacts with dynamic-range-limit:
              no-limit. On HDR displays, values above 1.0 should produce visibly
              brighter output.
            </p>
            <div
              className="flex flex-col gap-3"
              style={extStyle({ dynamicRangeLimit: 'no-limit' })}
            >
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
            <p className="text-text-muted text-xs">
              Tests oklch lightness values beyond 1.0 under no-limit. On HDR
              displays, L &gt; 1.0 should be brighter than standard white.
            </p>
            <div
              className="flex gap-1"
              style={extStyle({ dynamicRangeLimit: 'no-limit' })}
            >
              {[
                0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,
                1.3, 1.4, 1.5,
              ].map((l) => (
                <div
                  key={l}
                  className="flex h-12 w-10 items-end justify-center rounded pb-1 text-[8px]"
                  style={{
                    backgroundColor: `oklch(${l} 0 0)`,
                    color: l > 0.6 ? 'black' : 'white',
                  }}
                >
                  {l.toFixed(1)}
                </div>
              ))}
            </div>
            <p className="text-text-muted text-xs">
              Same with chroma (hue 10, brand red):
            </p>
            <div
              className="flex gap-1"
              style={extStyle({ dynamicRangeLimit: 'no-limit' })}
            >
              {[
                0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,
                1.3, 1.4, 1.5,
              ].map((l) => (
                <div
                  key={l}
                  className="flex h-12 w-10 items-end justify-center rounded pb-1 text-[8px]"
                  style={{
                    backgroundColor: `oklch(${l} 0.15 10)`,
                    color: l > 0.6 ? 'black' : 'white',
                  }}
                >
                  {l.toFixed(1)}
                </div>
              ))}
            </div>
            <p className="text-text-muted text-xs">
              Same with chroma (hue 153, success green):
            </p>
            <div
              className="flex gap-1"
              style={extStyle({ dynamicRangeLimit: 'no-limit' })}
            >
              {[
                0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1, 1.2,
                1.3, 1.4, 1.5,
              ].map((l) => (
                <div
                  key={l}
                  className="flex h-12 w-10 items-end justify-center rounded pb-1 text-[8px]"
                  style={{
                    backgroundColor: `oklch(${l} 0.22 153)`,
                    color: l > 0.6 ? 'black' : 'white',
                  }}
                >
                  {l.toFixed(1)}
                </div>
              ))}
            </div>
          </Section>

          {/* === 6. Panel Depth Simulation === */}
          <Section title="6. Panel Depth Hierarchy">
            <p className="text-text-muted text-xs">
              Simulates the active/inactive panel glow effect. The
              &quot;Active&quot; panel uses dynamic-range-limit: no-limit with
              the HDR glow tokens. The &quot;Inactive&quot; panel uses
              dynamic-range-limit: standard with a receding shadow.
            </p>
            <div className="bg-background-dark flex gap-4 rounded-xl p-6">
              <div
                className="flex h-32 flex-1 flex-col items-center justify-center rounded-lg border"
                style={extStyle({
                  dynamicRangeLimit: 'no-limit',
                  borderColor:
                    'var(--hdr-glow-active-border, var(--color-ui-outline-active))',
                  boxShadow:
                    '0 0 10px 2px var(--hdr-glow-active-shadow, transparent)',
                  background: 'var(--color-background-primary)',
                })}
              >
                <span className="text-text-primary text-sm font-semibold">
                  Active Panel
                </span>
                <span className="text-text-muted text-[10px]">
                  no-limit + glow
                </span>
              </div>
              <div
                className="flex h-32 flex-1 flex-col items-center justify-center rounded-lg border"
                style={extStyle({
                  dynamicRangeLimit: 'standard',
                  borderColor: 'var(--color-window-border)',
                  boxShadow:
                    '0 2px 8px var(--hdr-shadow-recede, oklch(0 0 0 / 0.3))',
                  background: 'var(--color-background-primary)',
                })}
              >
                <span className="text-text-secondary text-sm">
                  Inactive Panel
                </span>
                <span className="text-text-muted text-[10px]">
                  standard + recede
                </span>
              </div>
              <div
                className="flex h-32 flex-1 flex-col items-center justify-center rounded-lg border"
                style={extStyle({
                  dynamicRangeLimit: 'standard',
                  borderColor: 'var(--color-window-border)',
                  background: 'var(--color-background-primary)',
                })}
              >
                <span className="text-text-secondary text-sm">
                  Inactive (no shadow)
                </span>
                <span className="text-text-muted text-[10px]">
                  standard, flat
                </span>
              </div>
            </div>
          </Section>

          {/* === 7. Text Legibility on Colored Backgrounds === */}
          <Section title="7. Text Legibility">
            <p className="text-text-muted text-xs">
              White text at different sizes on various backgrounds. Tests
              readability for bar items, status labels, and panel text.
            </p>
            <div className="flex flex-col gap-3">
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
            <p className="text-text-muted text-xs">
              Preview of the --hdr-glow-* tokens. These only resolve inside
              @media (dynamic-range: high). On SDR displays they fall back to
              transparent.
            </p>
            <div className="flex flex-wrap gap-3">
              <Swatch
                label="glow-focus"
                style={{
                  backgroundColor:
                    'var(--hdr-glow-focus, oklch(0.4 0 0 / 0.3))',
                }}
              />
              <Swatch
                label="glow-active-border"
                style={{
                  backgroundColor:
                    'var(--hdr-glow-active-border, oklch(0.4 0 0 / 0.3))',
                }}
              />
              <Swatch
                label="glow-active-shadow"
                style={{
                  backgroundColor:
                    'var(--hdr-glow-active-shadow, oklch(0.4 0 0 / 0.3))',
                }}
              />
              <Swatch
                label="shadow-recede"
                style={{
                  backgroundColor:
                    'var(--hdr-shadow-recede, oklch(0.4 0 0 / 0.3))',
                }}
              />
            </div>
          </Section>

          {/* === 9. Side-by-Side Chroma Comparison === */}
          <Section title="9. Chroma Comparison (SDR vs P3 target)">
            <p className="text-text-muted text-xs">
              Left column: SDR oklch values. Right column: P3 target values
              (hardcoded). Your display renders whatever it can.
            </p>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <h3 className="text-text-muted mb-2 text-center text-[10px] font-semibold uppercase">
                  SDR Chroma
                </h3>
                <div className="flex flex-col gap-1">
                  {[
                    ['brand', 'oklch(0.7717 0.1355 16.5)'],
                    ['primary', 'oklch(0.55 0.20 16.5)'],
                    ['success', 'oklch(0.8771 0.2241 153.23)'],
                    ['warning', 'oklch(0.8405 0.0985 55.41)'],
                    ['error', 'oklch(0.7703 0.1356 20.68)'],
                    ['logo-blue', 'oklch(0.7126 0.1209 226.36)'],
                    ['logo-pink', 'oklch(0.6241 0.1659 336.54)'],
                  ].map(([name, color]) => (
                    <div key={name} className="flex items-center gap-2">
                      <div
                        className="h-6 w-12 rounded"
                        style={{ backgroundColor: color }}
                      />
                      <span className="text-text-muted font-mono text-[9px]">
                        {name}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
              <div>
                <h3 className="text-text-muted mb-2 text-center text-[10px] font-semibold uppercase">
                  P3 Chroma (boosted)
                </h3>
                <div className="flex flex-col gap-1">
                  {[
                    ['brand', 'oklch(0.7717 0.18 16.5)'],
                    ['primary', 'oklch(0.55 0.26 16.5)'],
                    ['success', 'oklch(0.8771 0.28 153.23)'],
                    ['warning', 'oklch(0.8405 0.14 55.41)'],
                    ['error', 'oklch(0.7104 0.17 18.72)'],
                    ['logo-blue', 'oklch(0.7057 0.12 220.43)'],
                    ['logo-pink', 'oklch(0.5913 0.2 330.41)'],
                  ].map(([name, color]) => (
                    <div key={name} className="flex items-center gap-2">
                      <div
                        className="h-6 w-12 rounded"
                        style={{ backgroundColor: color }}
                      />
                      <span className="text-text-muted font-mono text-[9px]">
                        {name}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </Section>

          {/* === 10. Window Glow Hover Test === */}
          <Section title="10. Window Glow Hover Test">
            <p className="text-text-muted text-xs">
              Hover over these boxes to see the HDR window glow effect. On SDR
              displays, the hdr-window-glow class has no effect.
            </p>
            <div className="bg-background-dark flex gap-4 rounded-xl p-6">
              <div className="hdr-window-glow flex h-24 flex-1 items-center justify-center rounded-lg border border-[var(--color-window-border)] bg-[var(--color-background-primary)]">
                <span className="text-text-secondary text-xs">
                  Hover me (window glow)
                </span>
              </div>
              <div className="hdr-window-glow flex h-24 flex-1 items-center justify-center rounded-lg border border-[var(--color-ui-outline)] bg-[var(--color-background-secondary)]">
                <span className="text-text-secondary text-xs">
                  Hover me (outline glow)
                </span>
              </div>
              <div className="flex h-24 flex-1 items-center justify-center rounded-lg border border-[var(--color-window-border)] bg-[var(--color-background-primary)]">
                <span className="text-text-secondary text-xs">
                  No glow (control)
                </span>
              </div>
            </div>
          </Section>

          {/* === 11. Status Flash Test === */}
          <Section title="11. Status Flash Test">
            <p className="text-text-muted text-xs">
              Click the buttons to trigger the HDR flash animation on the
              indicator. On SDR displays, the class has no visual effect.
            </p>
            <StatusFlashDemo />
          </Section>

          <div className="text-text-muted border-t border-white/10 pt-4 pb-8 text-xs">
            <p>
              <strong>Tips:</strong> To test HDR, use a display that supports it
              (MacBook Pro XDR, external HDR monitor). Set display brightness
              below max — HDR headroom only exists when the display has room to
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
    <div className="bg-background-card flex flex-wrap items-center gap-4 rounded-lg p-4">
      {statuses.map((status) => (
        <button
          key={status}
          className="hdr-status-flash cursor-pointer rounded border border-white/10 px-3 py-2 font-mono text-xs font-bold"
          style={{ color: statusColors[status] }}
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
      <span className="text-text-muted text-[10px]">
        Click to trigger flash
      </span>
    </div>
  )
}
