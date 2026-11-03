import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 4)` },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 32)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule4: { minWidth: 0, flex: 1, overflowWrap: 'break-word' },
  rule5: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule6: { color: themeColors['--color-destructive'] },
  rule7: { color: themeColors['--color-warning'] },
  rule8: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground']} 90%, transparent)`,
  },
})

// Fact is one labeled reading in an inspector.
export interface Fact {
  label: string
  value: string | number | bigint | boolean | null | undefined
  mono?: boolean
  tone?: 'warning' | 'error'
}

// Facts renders labeled readings as a definition list. Readings with no value
// are omitted so the list shows only what the system reported; an empty list
// renders the fallback line instead.
export function Facts({
  facts,
  empty = 'Nothing reported.',
}: {
  facts: Fact[]
  empty?: string
}) {
  const present = facts.filter(
    (fact) => fact.value !== '' && fact.value != null,
  )
  if (!present.length) {
    return <p sx={stylexStyles.rule1}>{empty}</p>
  }

  return (
    <dl sx={stylexStyles2.rule1}>
      {present.map((fact) => (
        <div
          key={fact.label}
          sx={[nativeChildSpacing.s1_5, stylexStyles.rule2]}
        >
          <dt sx={stylexStyles.rule3}>{fact.label}</dt>
          <dd
            sx={[
              stylexStyles.rule4,
              !!fact.mono && stylexStyles.rule5,
              fact.tone === 'error'
                ? stylexStyles.rule6
                : fact.tone === 'warning'
                  ? stylexStyles.rule7
                  : stylexStyles.rule8,
            ]}
          >
            {typeof fact.value === 'boolean'
              ? fact.value
                ? 'Yes'
                : 'No'
              : String(fact.value)}
          </dd>
        </div>
      ))}
    </dl>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
