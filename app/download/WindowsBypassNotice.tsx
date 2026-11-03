import { useState } from 'react'
import { LuPlus, LuShieldAlert } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    listStyleType: 'decimal',
    marginLeft: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
  },
  rule3: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    display: 'flex',
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-brand'],
      },
    },
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule6: {
    borderRadius: themeRadii['--radius-md'],
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'center',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule7: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 12%, transparent)`,
    color: themeColors['--color-brand'],
    rotate: '45deg',
  },
  rule8: {
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          `color-mix(in oklab, ${themeColors['--color-brand']} 8%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker2)]:
          themeColors['--color-brand'],
      },
    },
  },
  rule9: {
    '--tw-duration': '.3s',
    '--tw-ease': 'var(--ease-in-out)',
    display: 'grid',
    transitionDuration: '.3s',
    transitionProperty: 'all',
    transitionTimingFunction: 'var(--ease-in-out)',
  },
  rule10: { gridTemplateRows: '1fr', opacity: 1 },
  rule11: { gridTemplateRows: '0fr', opacity: 0 },
  rule12: { overflow: 'hidden' },
  rule13: {
    '--tw-leading': 'var(--leading-relaxed)',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule14: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule15: { color: themeColors['--color-foreground'] },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    WebkitUserSelect: 'none',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-start',
    cursor: 'pointer',
    display: 'flex',
    justifyContent: 'space-between',
    textAlign: 'left',
    userSelect: 'none',
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-brand'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// WindowsBypassNotice renders a collapsible "How to run on Windows" block
// describing how to bypass SmartScreen for the interim unsigned build.
// Collapsed by default. Keyboard accessible as a <button> disclosure.
export function WindowsBypassNotice() {
  const [isOpen, setOpen] = useState(false)

  return (
    <div
      sx={[
        stylexStyles3.rule1,
        isOpen ? stylexStyles3.rule2 : stylexStyles3.rule3,
      ]}
    >
      <button
        type="button"
        aria-expanded={isOpen}
        onClick={() => setOpen((v) => !v)}
        sx={[stylexStyles2.rule1, groupMarker]}
      >
        <div sx={stylexStyles3.rule4}>
          <LuShieldAlert sx={stylexStyles.rule1} />
          <h3 sx={stylexStyles3.rule5}>How to run on Windows</h3>
        </div>
        <div
          sx={[
            stylexStyles3.rule6,
            isOpen ? stylexStyles3.rule7 : stylexStyles3.rule8,
          ]}
        >
          <LuPlus sx={stylexStyles.rule2} />
        </div>
      </button>
      <div
        sx={[
          stylexStyles3.rule9,
          isOpen ? stylexStyles3.rule10 : stylexStyles3.rule11,
        ]}
      >
        <div sx={stylexStyles3.rule12}>
          <div sx={stylexStyles3.rule13}>
            <ol sx={stylexStyles4.rule1}>
              <li sx={nativeChildSpacing.s2}>
                Run the downloaded <span sx={stylexStyles3.rule14}>.exe</span>.
                If SmartScreen shows{' '}
                <strong sx={stylexStyles3.rule15}>
                  &quot;Windows protected your PC&quot;
                </strong>
                , click <strong sx={stylexStyles3.rule15}>More info</strong>,
                then <strong sx={stylexStyles3.rule15}>Run anyway</strong>.
              </li>
              <li sx={nativeChildSpacing.s2}>
                Spacewave installs itself, so this is only needed once.
              </li>
            </ol>
            <p sx={stylexStyles3.rule16}>
              The Windows build is unsigned during Microsoft&apos;s code-signing
              identity validation. The SmartScreen prompt disappears once
              validation completes.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})
