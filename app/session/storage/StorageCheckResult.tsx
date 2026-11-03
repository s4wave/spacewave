import { LuCircleCheck, LuTriangleAlert } from '@s4wave/web/images/icons.js'

import { CopyButton } from '@s4wave/web/ui/CopyButton.js'

import type { StorageCheckView, StorageCorsRule } from './storage-backend.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-success'],
  },
  rule2: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: { minWidth: 0, flex: 1 },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    maxHeight: `calc(${themeLayout['--spacing']} * 48)`,
    overflow: 'auto',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-snug)',
    '--tw-leading': 'var(--leading-snug)',
    color: themeColors['--color-foreground'],
  },
  rule12: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule13: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
})

interface StorageCheckResultProps {
  check: StorageCheckView
  // corsRule is shown when the check suggests the bucket lacks it.
  corsRule?: StorageCorsRule
}

// StorageCheckResult shows a bucket check as a result line, or as the
// failure's cause and the change that fixes it.
export function StorageCheckResult({
  sx,
  check,
  corsRule,
}: StorageCheckResultProps & { sx?: stylex.StyleXStyles; className?: never }) {
  if (check.ok) {
    return (
      <p sx={[stylexStyles2.rule1, sx]} role="status">
        <LuCircleCheck sx={stylexStyles.rule1} aria-hidden="true" />
        Connected. A test object was written, read, and deleted.
        <span sx={stylexStyles2.rule2}>Holds {check.usage}.</span>
      </p>
    )
  }

  return (
    <div sx={[stylexStyles2.rule3, sx]} role="alert">
      <div sx={stylexStyles2.rule4}>
        <LuTriangleAlert sx={stylexStyles.rule2} aria-hidden="true" />
        <div sx={stylexStyles2.rule5}>
          <h3 sx={stylexStyles2.rule6}>{check.title}</h3>
          <p sx={stylexStyles2.rule7}>{check.action}</p>

          {check.showCors && corsRule && (
            <div sx={stylexStyles2.rule8}>
              <div sx={stylexStyles2.rule9}>
                <p sx={stylexStyles2.rule10}>{corsRule.where}</p>
                <CopyButton text={corsRule.text} label="Copy CORS rule" />
              </div>
              <pre sx={stylexStyles2.rule11}>{corsRule.text}</pre>
            </div>
          )}

          {check.detail && (
            <details sx={stylexStyles2.rule8}>
              <summary sx={stylexStyles2.rule12}>Details</summary>
              <p sx={stylexStyles2.rule13}>{check.detail}</p>
            </details>
          )}
        </div>
      </div>
    </div>
  )
}
