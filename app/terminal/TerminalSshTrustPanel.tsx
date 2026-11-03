import { LuCheck, LuShieldQuestion, LuX } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import type { TerminalFrame } from '@s4wave/sdk/terminal/terminal.pb.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({ rule1: { minWidth: 0 } })

const stylexStyles3 = stylex.create({
  rule1: {
    margin: 0,
    borderColor: `color-mix(in oklab, ${themeColors['--color-warning']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 80%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    position: 'static',
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 48rem)': 'flex-start' },
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 48rem)': 'row' },
    justifyContent: {
      default: null,
      '@media (min-width: 48rem)': 'space-between',
    },
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-warning'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule4: {
    gap: themeLayout['--spacing'],
    display: 'grid',
    fontSize: themeFonts['--text-xs'],
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 48rem)': 'minmax(8rem, .35fr) minmax(0, 1fr)',
    },
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    WebkitUserSelect: 'none',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    userSelect: 'none',
  },
  rule6: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    wordBreak: 'break-all',
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexShrink: 0,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: { WebkitUserSelect: 'none', userSelect: 'none' },
})

export interface TerminalSshTrustPanelProps {
  challenge: TerminalFrame
  onRespond: (accepted: boolean) => void
}

export function TerminalSshTrustPanel({
  challenge,
  onRespond,
}: TerminalSshTrustPanelProps) {
  return (
    <dialog open aria-label="SSH host key trust" sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <div sx={stylexStyles4.rule1}>
          <div sx={[nativeChildSpacing.s2, stylexStyles3.rule3]}>
            <LuShieldQuestion sx={stylexStyles2.rule1} />
            Confirm SSH host key
          </div>
          <div sx={[nativeChildSpacing.s2, stylexStyles3.rule4]}>
            <span sx={stylexStyles3.rule5}>Host</span>
            <span sx={stylexStyles3.rule6}>
              {challenge.sshTrustHost || 'unknown'}
            </span>
            <span sx={stylexStyles3.rule5}>Algorithm</span>
            <span sx={stylexStyles3.rule6}>
              {challenge.sshTrustAlgorithm || 'unknown'}
            </span>
            <span sx={stylexStyles3.rule5}>SHA256</span>
            <span sx={stylexStyles3.rule6}>
              {challenge.sshTrustSha256Fingerprint || 'unknown'}
            </span>
            <span sx={stylexStyles3.rule5}>Public key</span>
            <span sx={stylexStyles3.rule6}>
              {challenge.sshTrustPublicKey || 'unknown'}
            </span>
          </div>
        </div>
        <div sx={stylexStyles3.rule7}>
          <Button
            variant="brandOutline"
            size="brandWide"
            sx={stylexStyles.rule1}
            onClick={() => onRespond(true)}
            type="button"
          >
            <LuCheck sx={stylexStyles2.rule2} />
            Trust
          </Button>
          <DashboardButton
            icon={<LuX sx={stylexStyles2.rule2} />}
            onClick={() => onRespond(false)}
          >
            Reject
          </DashboardButton>
        </div>
      </div>
    </dialog>
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
